//	Public interfaces:
//		ConnectionBlocked()
//		ConnectionUnlocked()
//		ConnectionClosed()
//		unlock_notify()

//	Head of a linked list of all sqlite3 objects created by this process for which either sqlite3.pBlockingConnection or sqlite3.pUnlockConnection is not nil. This variable may only accessed while the STATIC_MASTER mutex is held.

var sqlite3BlockedList *sqlite3

//	Remove connection db from the blocked connections list. If connection db is not currently a part of the list, this function is a no-op.
static void removeFromBlockedList(sqlite3 *db){
  sqlite3 **pp;
  for(pp=&sqlite3BlockedList; *pp; pp = &(*pp).NextBlocked){
    if( *pp==db ){
      *pp = (*pp).NextBlocked;
      break;
    }
  }
}

//	Add connection db to the blocked connections list. It is assumed that it is not already a part of the list.
static void addToBlockedList(sqlite3 *db){
  sqlite3 **pp;
  for(
    pp=&sqlite3BlockedList; 
    *pp && (*pp).xUnlockNotify!=db.xUnlockNotify; 
    pp=&(*pp).NextBlocked
  );
  db.NextBlocked = *pp;
  *pp = db;
}

//	Obtain the STATIC_MASTER mutex.
func CriticalSection(mutex int, f func()) {
	defer sqlite3MutexAlloc(mutex).Unlock()
	sqlite3MutexAlloc(mutex).Lock()
	f()
}

//	Register an unlock-notify callback.
//	This is called after connection "db" has attempted some operation but has received an SQLITE_LOCKED error because another connection (call it pOther) in the same process was busy using the same shared cache. pOther is found by looking at db.pBlockingConnection.
//	If there is no blocking connection, the callback is invoked immediately, before this routine returns.
//	If pOther is already blocked on db, then report SQLITE_LOCKED, to indicate a deadlock.
//	Otherwise, make arrangements to invoke xNotify when pOther drops its locks.
//	Each call to this routine overrides any prior callbacks registered on the same "db". If xNotify == 0 then any prior callbacks are immediately cancelled.
func (db *sqlite3) unlock_notify(xNotify func([]interface{}, int), pArg interface{}) (rc int) {
	rc = SQLITE_OK
	db.mutex.Lock()
	CriticalSection(SQLITE_MUTEX_STATIC_MASTER, func() {
		if xNotify == nil {
			removeFromBlockedList(db)
			db.pBlockingConnection = nil
			db.pUnlockConnection = nil
			db.xUnlockNotify = nil
			db.pUnlockArg = nil
		} else if db.pBlockingConnection == nil {
			//	The blocking transaction has been concluded. Or there never was a blocking transaction. In either case, invoke the notify callback immediately. 
			xNotify(&pArg, 1)
		} else {
			sqlite3 *p

			for(p=db.pBlockingConnection; p && p!=db; p=p.pUnlockConnection){}
				if p != nil {
				rc = SQLITE_LOCKED					//	Deadlock detected.
			} else {
				db.pUnlockConnection = db.pBlockingConnection
				db.xUnlockNotify = xNotify
				db.pUnlockArg = pArg
				removeFromBlockedList(db)
				addToBlockedList(db)
			}
		}
	})
	assert( !db.mallocFailed )
	if rc != SQLITE_OK {
		db.Error(rc, "database is deadlocked")
	} else {
		db.Error(rc, "")
	}
	db.mutex.Unlock()
	return
}

//	This function is called while stepping or preparing a statement associated with connection db. The operation will return SQLITE_LOCKED to the user because it requires a lock that will not be available until connection pBlocker concludes its current transaction.
func (db *sqlite3) ConnectionBlocked(pBlocker *sqlite3) {
	CriticalSection(SQLITE_MUTEX_STATIC_MASTER, func() {
		if db.pBlockingConnection == nil && db.pUnlockConnection == nil {
			addToBlockedList(db)
		}
		db.pBlockingConnection = pBlocker
	})
}

//	This function is called when the transaction opened by database db has just finished. Locks held by database connection db have been released.
//	This function loops through each entry in the blocked connections list and does the following:
//		1) If the sqlite3.pBlockingConnection member of a list entry is set to db, then set pBlockingConnection = 0.
//		2) If the sqlite3.pUnlockConnection member of a list entry is set to db, then invoke the configured unlock-notify callback and set pUnlockConnection = 0.
//		3) If the two steps above mean that pBlockingConnection == 0 and pUnlockConnection == 0, remove the entry from the blocked connections list.
func (db *sqlite3) ConnectionUnlocked() {
	void (*xUnlockNotify)(void **, int) = 0; /* Unlock-notify cb to invoke */
	int nArg = 0;                            /* Number of entries in aArg[] */
	sqlite3 **pp;                            /* Iterator variable */
	void **aArg;               /* Arguments to the unlock callback */
	void **aDyn = 0;           /* Dynamically allocated space for aArg[] */
	void *aStatic[16];         /* Starter space for aArg[].  No malloc required */

	aArg = aStatic
	CriticalSection(SQLITE_MUTEX_STATIC_MASTER, func() {
		//	This loop runs once for each entry in the blocked-connections list.
		for(pp=&sqlite3BlockedList; *pp; /* no-op */ ){
			sqlite3 *p = *pp;

			//	Step 1.
			if p.pBlockingConnection == db {
				p.pBlockingConnection = nil
			}

			//	Step 2.
			if p.pUnlockConnection == db {
				assert( p.xUnlockNotify )
				if p.xUnlockNotify != xUnlockNotify && nArg != 0 {
					xUnlockNotify(aArg, nArg)
					nArg = 0
				}

				assert( aArg == aDyn || (aDyn == 0 && aArg == aStatic) )
				assert( nArg <= len(aStatic) || aArg == aDyn )
				if (aDyn == nil && nArg == len(aStatic)) || (aDyn == nil && nArg == len(aDyn)) {
					//	The aArg[] array needs to grow.
					void **pNew = (void **)sqlite3Malloc(nArg * sizeof(void *)*2)
					if pNew != nil {
						memcpy(pNew, aArg, nArg*sizeof(void *))
						aDyn = aArg = pNew
					} else {
						//	This occurs when the array of context pointers that need to be passed to the unlock-notify callback is larger than the aStatic[] array allocated on the stack and the attempt to allocate a larger array from the heap has failed.
						//	This is a difficult situation to handle. Returning an error code to the caller is insufficient, as even if an error code is returned the transaction on connection db will still be closed and the unlock-notify callbacks on blocked connections will go unissued. This might cause the application to wait indefinitely for an unlock-notify callback that will never arrive.
						//	Instead, invoke the unlock-notify callback with the context array already accumulated. We can then clear the array and begin accumulating any further context pointers without requiring any dynamic allocation. This is sub-optimal because it means that instead of one callback with a large array of context pointers the application will receive two or more callbacks with smaller arrays of context pointers, which will reduce the applications ability to prioritize multiple connections. But it is the best that can be done under the circumstances.
						xUnlockNotify(aArg, nArg)
						nArg = 0
					}
				}

				aArg[nArg] = p.pUnlockArg
				nArg++
				xUnlockNotify = p.xUnlockNotify
				p.pUnlockConnection = nil
				p.xUnlockNotify = nil
				p.pUnlockArg = nil
			}

			//	Step 3.
			if p.pBlockingConnection == nil && p.pUnlockConnection == nil {
				//	Remove connection p from the blocked connections list.
				*pp = p.NextBlocked
				p.NextBlocked = nil
			} else {
				pp = &p.NextBlocked
			}
		}
		if nArg != 0 {
			xUnlockNotify(aArg, nArg)
		}
		aDyn = nil
	})
}

//	This is called when the database connection passed as an argument is being closed. The connection is removed from the blocked list.
func (db *sqlite) ConnectionClosed() {
	db.ConnectionUnlocked()
	CriticalSection(SQLITE_MUTEX_STATIC_MASTER, func() {
		removeFromBlockedList(db)
	})
}