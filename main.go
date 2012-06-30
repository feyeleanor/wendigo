/* Main file for the SQLite library.  The routines in this file
** implement the programmer interface to the library.  Routines in
** other files are for internal use by SQLite and should not be
** accessed by users of the library.
*/

#ifdef SQLITE_ENABLE_FTS3
/************** Include fts3.h in the middle of main.c ***********************/
/************** Begin file fts3.h ********************************************/
/* This header file is used by programs that want to link against the
** FTS3 library.  All it does is declare the sqlite3Fts3Init() interface.
*/

 int sqlite3Fts3Init(sqlite3 *db);

/************** End of fts3.h ************************************************/
/************** Continuing where we left off in main.c ***********************/
#endif
#ifdef SQLITE_ENABLE_RTREE
/************** Include rtree.h in the middle of main.c **********************/
/************** Begin file rtree.h *******************************************/
/* This header file is used by programs that want to link against the
** RTREE library.  All it does is declare the sqlite3RtreeInit() interface.
*/

 int sqlite3RtreeInit(sqlite3 *db);

/************** End of rtree.h ***********************************************/
/************** Continuing where we left off in main.c ***********************/
#endif

/* IMPLEMENTATION-OF: R-53536-42575 The sqlite3_libversion() function returns
** a pointer to the to the sqlite3_version[] string constant. 
*/
func const char *sqlite3_libversion(void){ return sqlite3_version; }

/* IMPLEMENTATION-OF: R-63124-39300 The sqlite3_sourceid() function returns a
** pointer to a string constant whose value is the same as the
** SQLITE_SOURCE_ID C preprocessor macro. 
*/
func const char *sqlite3_sourceid(void){ return SQLITE_SOURCE_ID; }

/* IMPLEMENTATION-OF: R-35210-63508 The sqlite3_libversion_number() function
** returns an integer equal to SQLITE_VERSION_NUMBER.
*/
func int sqlite3_libversion_number(void){ return SQLITE_VERSION_NUMBER; }

#if !defined(SQLITE_OMIT_TRACE) && defined(SQLITE_ENABLE_IOTRACE)
/*
** If the following function pointer is not NULL and if
** SQLITE_ENABLE_IOTRACE is enabled, then messages describing
** I/O active are written using this function.  These messages
** are intended for debugging activity only.
*/
 void (*sqlite3IoTrace)(const char*, ...) = 0;
#endif

/*
** If the following global variable points to a string which is the
** name of a directory, then that directory will be used to store
** temporary files.
**
** See also the "PRAGMA temp_store_directory" SQL command.
*/
func char *sqlite3_temp_directory = 0;

//	Initialize SQLite.  
//
//	This routine must be called to initialize the memory allocation, VFS, and mutex subsystems prior to doing any serious work with SQLite.
//
//	This routine is a no-op except on its very first call for the process, or for the first call after a call to Shutdown.
//
//	The first thread to call this routine runs the initialization to completion. If subsequent threads call this routine before the first
//	thread has finished the initialization process, then the subsequent threads must block until the first thread finishes with the initialization.
//
//	The first thread might call this routine recursively. Recursive calls to this routine should not block, of course. Otherwise the
//	initialization process would never complete.
//
//	Let X be the first thread to enter this routine. Let Y be some other thread. Then while the initial invocation of this routine by X is
//	incomplete, it is required that:
//
//		*  Calls to this routine from Y must block until the outer-most call by X completes.
//
//		*  Recursive calls to this routine from thread X return immediately without blocking.
func Initialize() (rc int) {
	pMaster		*sqlite3_mutex				//	The main static mutex

	//	If SQLite is already completely initialized, then this call to Initialize() should be a no-op. But the initialization must be complete. So isInit must not be set until the very end of this routine.
	if sqlite3GlobalConfig.isInit {
		return SQLITE_OK
	}

	//	Make sure the mutex subsystem is initialized. If unable to initialize the mutex subsystem, return early with the error. If the system is so sick that we are unable to allocate a mutex, there is not much SQLite is going to be able to do.
	//	The mutex subsystem must take care of serializing its own initialization.
	if rc = sqlite3MutexInit(); rc != SQLITE_OK {
		return rc
	}

	//	Initialize the malloc() system and the recursive pInitMutex mutex. This operation is protected by the STATIC_MASTER mutex. Note that MutexAlloc() is called for a static mutex prior to initializing the malloc subsystem - this implies that the allocation of a static mutex must not require support from the malloc subsystem.
	pMaster = sqlite3MutexAlloc(SQLITE_MUTEX_STATIC_MASTER)
	pMaster.Lock()
	sqlite3GlobalConfig.isMutexInit = true
	if !sqlite3GlobalConfig.isMallocInit {
		rc = sqlite3MallocInit()
	}
	if rc == SQLITE_OK {
		sqlite3GlobalConfig.isMallocInit = true
		if !sqlite3GlobalConfig.pInitMutex {
			sqlite3GlobalConfig.pInitMutex = sqlite3MutexAlloc(SQLITE_MUTEX_RECURSIVE)
			if sqlite3GlobalConfig.bCoreMutex && !sqlite3GlobalConfig.pInitMutex {
				rc = SQLITE_NOMEM
			}
		}
	}
	if rc == SQLITE_OK {
		sqlite3GlobalConfig.nRefInitMutex++
	}
	pMaster.Unlock()

	//	If rc is not SQLITE_OK at this point, then either the malloc subsystem could not be initialized or the system failed to allocate the pInitMutex mutex. Return an error in either case.
	if rc != SQLITE_OK {
		return rc
	}

	//	Do the rest of the initialization under the recursive mutex so that we will be able to handle recursive calls into Initialize(). The recursive calls normally come through sqlite3_os_init() when it invokes sqlite3_vfs_register(), but other recursive calls might also be possible.
	//	IMPLEMENTATION-OF: R-00140-37445 SQLite automatically serializes calls to the xInit method, so the xInit method need not be threadsafe.
	//	The following mutex is what serializes access to the appdef pcache xInit methods. The sqlite3_pcache_methods.xInit() all is embedded in the call to sqlite3PcacheInitialize().
	sqlite3GlobalConfig.pInitMutex.Lock()
	if !sqlite3GlobalConfig.isInit && !sqlite3GlobalConfig.inProgress {
		sqlite3GlobalFunctions = make(FunctionTable)
		sqlite3GlobalConfig.inProgress = true
		sqlite3RegisterGlobalFunctions()
		if !sqlite3GlobalConfig.isPCacheInit {
			rc = sqlite3PcacheInitialize()
		}
		if rc == SQLITE_OK {
			sqlite3GlobalConfig.isPCacheInit = true
			rc = sqlite3OsInit()
		}
		if rc == SQLITE_OK {
			sqlite3PCacheBufferSetup( sqlite3GlobalConfig.pPage, sqlite3GlobalConfig.PageSize, sqlite3GlobalConfig.nPage)
			sqlite3GlobalConfig.isInit = true
		}
		sqlite3GlobalConfig.inProgress = false
	}
	sqlite3GlobalConfig.pInitMutex.Unlock()

	//	Go back under the static mutex and clean up the recursive mutex to prevent a resource leak.
	pMaster.Lock()
	sqlite3GlobalConfig.nRefInitMutex--
	if sqlite3GlobalConfig.nRefInitMutex <= 0 {
		assert( sqlite3GlobalConfig.nRefInitMutex == 0 )
		sqlite3GlobalConfig.pInitMutex.Free()
		sqlite3GlobalConfig.pInitMutex = nil
	}
	pMaster.Unlock()
	return rc
}

//	Undo the effects of Initialize(). Must not be called while there are outstanding database connections or memory allocations or while any part of SQLite is otherwise in use in any thread. This routine is not threadsafe. But it is safe to invoke this routine on when SQLite is already shut down. If SQLite is already shut down when this routine is invoked, then this routine is a harmless no-op.
func Shutdown() int {
	if sqlite3GlobalConfig.isInit {
		sqlite3_os_end()
		sqlite3_reset_auto_extension()
		sqlite3GlobalConfig.isInit = false
	}
	if sqlite3GlobalConfig.isPCacheInit {
		sqlite3PcacheShutdown()
		sqlite3GlobalConfig.isPCacheInit = false
	}
	if sqlite3GlobalConfig.isMallocInit {
		sqlite3MallocEnd()
		sqlite3GlobalConfig.isMallocInit = false
	}
	if sqlite3GlobalConfig.isMutexInit {
		sqlite3MutexEnd()
		sqlite3GlobalConfig.isMutexInit = false
	}
	return SQLITE_OK
}

//	This API allows applications to modify the global configuration of the SQLite library at run-time.
//
//	This routine should only be called when there are no outstanding database connections or memory allocations. This routine is not threadsafe. Failure to heed these warnings can lead to unpredictable behavior.
func int sqlite3_config(int op, ...){
  va_list ap;
  int rc = SQLITE_OK;

  /* sqlite3_config() shall return SQLITE_MISUSE if it is invoked while
  ** the SQLite library is in use. */
  if( sqlite3GlobalConfig.isInit ) return SQLITE_MISUSE_BKPT;

  va_start(ap, op);
  switch( op ){

    /* Mutex configuration options are only available in a threadsafe
    ** compile. 
    */
    case SQLITE_CONFIG_SINGLETHREAD: {
      /* Disable all mutexing */
      sqlite3GlobalConfig.bCoreMutex = 0;
      sqlite3GlobalConfig.bFullMutex = 0;
      break;
    }
    case SQLITE_CONFIG_MULTITHREAD: {
      /* Disable mutexing of database connections */
      /* Enable mutexing of core data structures */
      sqlite3GlobalConfig.bCoreMutex = 1;
      sqlite3GlobalConfig.bFullMutex = 0;
      break;
    }
    case SQLITE_CONFIG_SERIALIZED: {
      /* Enable all mutexing */
      sqlite3GlobalConfig.bCoreMutex = 1;
      sqlite3GlobalConfig.bFullMutex = 1;
      break;
    }
    case SQLITE_CONFIG_MUTEX: {
      /* Specify an alternative mutex implementation */
      sqlite3GlobalConfig.mutex = *va_arg(ap, sqlite3_mutex_methods*);
      break;
    }
    case SQLITE_CONFIG_GETMUTEX: {
      /* Retrieve the current mutex implementation */
      *va_arg(ap, sqlite3_mutex_methods*) = sqlite3GlobalConfig.mutex;
      break;
    }

    case SQLITE_CONFIG_MALLOC: {
      /* Specify an alternative malloc implementation */
      sqlite3GlobalConfig.m = *va_arg(ap, sqlite3_mem_methods*);
      break;
    }
    case SQLITE_CONFIG_GETMALLOC: {
      /* Retrieve the current malloc() implementation */
      if( sqlite3GlobalConfig.m.xMalloc==0 ) sqlite3MemSetDefault();
      *va_arg(ap, sqlite3_mem_methods*) = sqlite3GlobalConfig.m;
      break;
    }
    case SQLITE_CONFIG_MEMSTATUS: {
      /* Enable or disable the malloc status collection */
      sqlite3GlobalConfig.bMemstat = va_arg(ap, int);
      break;
    }
    case SQLITE_CONFIG_SCRATCH: {
      /* Designate a buffer for scratch memory space */
      sqlite3GlobalConfig.pScratch = va_arg(ap, void*);
      sqlite3GlobalConfig.szScratch = va_arg(ap, int);
      sqlite3GlobalConfig.nScratch = va_arg(ap, int);
      break;
    }
    case SQLITE_CONFIG_PAGECACHE: {
      /* Designate a buffer for page cache memory space */
      sqlite3GlobalConfig.pPage = va_arg(ap, void*);
      sqlite3GlobalConfig.PageSize = va_arg(ap, int);
      sqlite3GlobalConfig.nPage = va_arg(ap, int);
      break;
    }

    case SQLITE_CONFIG_PCACHE: {
      /* no-op */
      break;
    }
    case SQLITE_CONFIG_GETPCACHE: {
      /* now an error */
      rc = SQLITE_ERROR;
      break;
    }

    case SQLITE_CONFIG_PCACHE2: {
      /* Specify an alternative page cache implementation */
      sqlite3GlobalConfig.pcache2 = *va_arg(ap, sqlite3_pcache_methods2*);
      break;
    }
    case SQLITE_CONFIG_GETPCACHE2: {
      if( sqlite3GlobalConfig.pcache2.xInit==0 ){
        sqlite3PCacheSetDefault();
      }
      *va_arg(ap, sqlite3_pcache_methods2*) = sqlite3GlobalConfig.pcache2;
      break;
    }

    case SQLITE_CONFIG_LOOKASIDE: {
      sqlite3GlobalConfig.szLookaside = va_arg(ap, int);
      sqlite3GlobalConfig.nLookaside = va_arg(ap, int);
      break;
    }
    
    /* Record a pointer to the logger funcction and its first argument.
    ** The default is NULL.  Logging is disabled if the function pointer is
    ** NULL.
    */
    case SQLITE_CONFIG_LOG: {
      /* MSVC is picky about pulling func ptrs from va lists.
      ** http://support.microsoft.com/kb/47961
      ** sqlite3GlobalConfig.xLog = va_arg(ap, void(*)(void*,int,const char*));
      */
      typedef void(*LOGFUNC_t)(void*,int,const char*);
      sqlite3GlobalConfig.xLog = va_arg(ap, LOGFUNC_t);
      sqlite3GlobalConfig.pLogArg = va_arg(ap, void*);
      break;
    }

    case SQLITE_CONFIG_URI: {
      sqlite3GlobalConfig.bOpenUri = va_arg(ap, int);
      break;
    }

    default: {
      rc = SQLITE_ERROR;
      break;
    }
  }
  va_end(ap);
  return rc;
}

/*
** Set up the lookaside buffers for a database connection.
** Return SQLITE_OK on success.  
** If lookaside is already active, return SQLITE_BUSY.
**
** The sz parameter is the number of bytes in each lookaside slot.
** The cnt parameter is the number of slots.  If pStart is NULL the
** space for the lookaside memory is obtained from sqlite3_malloc().
** If pStart is not NULL then it is sz*cnt bytes of memory to use for
** the lookaside memory.
*/
static int setupLookaside(sqlite3 *db, void *pBuf, int sz, int cnt){
  void *pStart;
  if( db.lookaside.nOut ){
    return SQLITE_BUSY;
  }
  /* Free any existing lookaside buffer for this handle before
  ** allocating a new one so we don't have to have space for 
  ** both at the same time.
  */
  if( db.lookaside.bMalloced ){
    db.lookaside.pStart = nil
  }
  //	The size of a lookaside slot after ROUNDDOWN(x, 8) needs to be larger than a pointer to be useful.
  sz = ROUNDDOWN(sz, 8);  /* IMP: R-33038-09382 */
  if( sz<=(int)sizeof(LookasideSlot*) ) sz = 0;
  if( cnt<0 ) cnt = 0;
  if( sz==0 || cnt==0 ){
    sz = 0;
    pStart = 0;
  }else if( pBuf==0 ){
    pStart = sqlite3Malloc( sz*cnt );  /* IMP: R-61949-35727 */
    if( pStart ) cnt = sqlite3MallocSize(pStart)/sz;
  }else{
    pStart = pBuf;
  }
  db.lookaside.pStart = pStart;
  db.lookaside.pFree = 0;
  db.lookaside.sz = (uint16)sz;
  if( pStart ){
    int i;
    LookasideSlot *p;
    assert( sz > (int)sizeof(LookasideSlot*) );
    p = (LookasideSlot*)pStart;
    for(i=cnt-1; i>=0; i--){
      p.Next = db.lookaside.pFree;
      db.lookaside.pFree = p;
      p = (LookasideSlot*)&((byte*)p)[sz];
    }
    db.lookaside.pEnd = p;
    db.lookaside.bEnabled = 1;
    db.lookaside.bMalloced = pBuf==0 ?1:0;
  }else{
    db.lookaside.pEnd = 0;
    db.lookaside.bEnabled = 0;
    db.lookaside.bMalloced = 0;
  }
  return SQLITE_OK;
}

/*
** Return the mutex associated with a database connection.
*/
func sqlite3_mutex *sqlite3_db_mutex(sqlite3 *db){
  return db.mutex;
}

//	Free up as much memory as we can from the given database connection.
func (db *sqlite3) db_release_memory() int {
	db.mutex.Lock()
	db.LockAll()
	for i, _ := range db.Databases {
		if pBt := db.Databases[i].pBt; pBt != nil {
			sqlite3PagerShrink(pBt.Pager())
		}
	}
	db.LeaveBtreeAll()
	db.mutex.Unlock()
	return SQLITE_OK
}

/*
** Configuration settings for an individual database connection
*/
func int sqlite3_db_config(sqlite3 *db, int op, ...){
  va_list ap;
  int rc;
  va_start(ap, op);
  switch( op ){
    case SQLITE_DBCONFIG_LOOKASIDE: {
      void *pBuf = va_arg(ap, void*); /* IMP: R-26835-10964 */
      int sz = va_arg(ap, int);       /* IMP: R-47871-25994 */
      int cnt = va_arg(ap, int);      /* IMP: R-04460-53386 */
      rc = setupLookaside(db, pBuf, sz, cnt);
      break;
    }
    default: {
      static const struct {
        int op;      /* The opcode */
        uint32 mask;    /* Mask of the bit in sqlite3.flags to set/clear */
      } aFlagOp[] = {
        { SQLITE_DBCONFIG_ENABLE_FKEY,    SQLITE_ForeignKeys    },
        { SQLITE_DBCONFIG_ENABLE_TRIGGER, SQLITE_EnableTrigger  },
      };
      uint i;
      rc = SQLITE_ERROR; /* IMP: R-42790-23372 */
      for(i=0; i<ArraySize(aFlagOp); i++){
        if( aFlagOp[i].op==op ){
          int onoff = va_arg(ap, int);
          int *pRes = va_arg(ap, int*);
          int oldFlags = db.flags;
          if( onoff>0 ){
            db.flags |= aFlagOp[i].mask;
          }else if( onoff==0 ){
            db.flags &= ~aFlagOp[i].mask;
          }
          if( oldFlags!=db.flags ){
            db.ExpirePreparedStatements()
          }
          if( pRes ){
            *pRes = (db.flags & aFlagOp[i].mask)!=0;
          }
          rc = SQLITE_OK;
          break;
        }
      }
      break;
    }
  }
  va_end(ap);
  return rc;
}


/*
** Return true if the buffer z[0..n-1] contains all spaces.
*/
static int allSpaces(const char *z, int n){
  while( n>0 && z[n-1]==' ' ){ n--; }
  return n==0;
}

/*
** This is the default collating function named "BINARY" which is always
** available.
**
** If the padFlag argument is not NULL then space padding at the end
** of strings is ignored.  This implements the RTRIM collation.
*/
static int binCollFunc(
  void *padFlag,
  int nKey1, const void *pKey1,
  int nKey2, const void *pKey2
){
  int rc, n;
  n = nKey1<nKey2 ? nKey1 : nKey2;
  rc = memcmp(pKey1, pKey2, n);
  if( rc==0 ){
    if( padFlag
     && allSpaces(((char*)pKey1)+n, nKey1-n)
     && allSpaces(((char*)pKey2)+n, nKey2-n)
    ){
      /* Leave rc unchanged at 0 */
    }else{
      rc = nKey1 - nKey2;
    }
  }
  return rc;
}

/*
** Another built-in collating sequence: NOCASE. 
**
** This collating sequence is intended to be used for "case independant
** comparison". SQLite's knowledge of upper and lower case equivalents
** extends only to the 26 characters used in the English language.
**
** At the moment there is only a UTF-8 implementation.
*/
static int nocaseCollatingFunc(
  void *NotUsed,
  int nKey1, const void *pKey1,
  int nKey2, const void *pKey2
){
  int r = CaseInsensitiveMatchN((const char *)pKey1, (const char *)pKey2, (nKey1 < nKey2) ? nKey1: nKey2);
  if( 0==r ){
    r = nKey1-nKey2;
  }
  return r;
}

/*
** Return the ROWID of the most recent insert
*/
func sqlite_int64 sqlite3_last_insert_rowid(sqlite3 *db){
  return db.lastRowid;
}

/*
** Return the number of changes in the most recent call to sqlite3_exec().
*/
func int sqlite3_changes(sqlite3 *db){
  return db.nChange;
}

/*
** Return the number of changes since the database handle was opened.
*/
func int sqlite3_total_changes(sqlite3 *db){
  return db.nTotalChange;
}

//	Close all open savepoints. This function only manipulates fields of the database handle object, it does not close any savepoints that may be open at the b-tree/pager level.
func (db *sqlite3) CloseSavepoints() {
	db.Savepoints = nil
	db.nStatement = 0;
	db.isTransactionSavepoint = false
}

/*
** Invoke the destructor function associated with FuncDef p, if any. Except,
** if this is not the last copy of the function, do not invoke it. Multiple
** copies of a single function are created when create_function() is called
** with SQLITE_ANY as the encoding.
*/
static void functionDestroy(sqlite3 *db, FuncDef *p){
  FuncDestructor *pDestructor = p.pDestructor;
  if( pDestructor ){
    pDestructor.nRef--;
    if( pDestructor.nRef==0 ){
      pDestructor.xDestroy(pDestructor.pUserData);
      pDestructor = nil
    }
  }
}

//	Close an existing SQLite database
func (db *sqlite3) Close() (rc int) {
	if db != nil {
		if db.SafetyCheckSickOrOk() {
			db.mutex.CriticalSection(func() {
				//	Force xDestroy calls on all virtual tables
				db.ResetInternalSchema(-1)

				//	If a transaction is open, the ResetInternalSchema() call above will not have called the xDisconnect() method on any virtual tables in the db.aVTrans[] array. The following VtabRollback() call will do so. We need to do this before the check for active SQL statements below, as the v-table implementation may be storing some prepared statements internally.
				db.VtabRollback()

				//	If there are any outstanding VMs, return SQLITE_BUSY.
				if db.pVdbe != nil {
					db.Error(SQLITE_BUSY, "unable to close due to unfinalised statements")
					db.mutex.Unlock()
					return SQLITE_BUSY
				}
				assert( db.SafetyCheckSickOrOk() )

				for i, _ := range db.Databases {
					pBt := db.Databases[i].pBt
					if pBt != nil && sqlite3BtreeIsInBackup(pBt) {
						db.Error(SQLITE_BUSY, "unable to close due to unfinished backup operation")
						db.mutex.Unlock()
						return SQLITE_BUSY
					}
				}

				//	Free any outstanding Savepoint structures.
				db.CloseSavepoints()

				for i, pDb := range db.Databases {
					if pDb.pBt != nil {
						sqlite3BtreeClose(pDb.pBt)
						pDb.pBt = nil
						if i != 1 {
							pDb.Schema = nil
						}
					}
				}
				db.ResetInternalSchema(-1)

				//	Tell the code in notify.c that the connection no longer holds any locks and does not require any further unlock-notify callbacks.
				db.ConnectionClosed()

				assert( len(db.Databases) <= 2 )
				assert( db.Databases == db.DatabasesStatic )
				for j := 0; j < len(db.aFunc.a); j++ {
					FuncDef *Next, *pHash, *p;
					for p = db.aFunc.a[j]; p; p = pHash {
						pHash = p.pHash
						for p != nil {
							functionDestroy(db, p)
							p = p.Next
						}
					}
				}
				for _, pColl := range db.Collations {
					//	Invoke any destructors registered for collation sequence user data.
					for j := 0; j < 3; j++ {
						if pColl[j].xDel {
							pColl[j].xDel(pColl[j].pUser)
						}
					}
				}
				db.Collations = make(map[string]*CollSeq)
				for _, pMod := range db.Modules {
					if pMod.xDestroy {
						pMod.xDestroy(pMod.Parameter)
					}
				}
				db.Modules := make(map[string]*Module)

				db.Error(SQLITE_OK, "")						//	Deallocates any cached error strings.
				if db.pErr != nil {
					db.pErr.Free()
				}
				sqlite3CloseExtensions(db)

				db.magic = SQLITE_MAGIC_ERROR

				//	The temp-database schema is allocated differently from the other schema objects (using sqliteMalloc() directly, instead of Btree::Schema()). So it needs to be freed here. Todo: Why not roll the temp schema into the same sqliteMalloc() as the one that allocates the database structure?
				db.Databases[1].Schema = nil
			})
			db.magic = SQLITE_MAGIC_CLOSED
			db.mutex.Free()
			assert( db.lookaside.nOut == 0 )			//	Fails on a lookaside memory leak
			if db.lookaside.bMalloced {
				db.lookaside.pStart = nil
			}
			db = nil
			rc = SQLITE_OK
		} else {
			rc = SQLITE_MISUSE_BKPT
		}
	}
	return
}

//	Rollback all database files. If tripCode is not SQLITE_OK, then any open cursors are invalidated ("tripped" - as in "tripping a circuit breaker") and made to return tripCode if there are any further attempts to use that cursor.
func (db *sqlite3) RollbackAll(tripCode int) {
	inTrans := false
	for i, _ := range db.Databases {
		if p := db.Databases[i].pBt; p != nil {
			if p.IsInTrans() {
				inTrans = true
			}
			p.Rollback(tripCode)
			db.Databases[i].inTrans = false
		}
	}
	db.VtabRollback()

	if db.flags & SQLITE_InternChanges {
		db.ExpirePreparedStatements()
		db.ResetInternalSchema(-1)
	}

	//	Any deferred constraint violations have now been resolved.
	db.nDeferredCons = 0

	//	If one has been configured, invoke the rollback-hook callback
	if db.xRollbackCallback && (inTrans || !db.autoCommit) {
		db.xRollbackCallback(db.pRollbackArg)
	}
}

/*
** Return a static string that describes the kind of error specified in the
** argument.
*/
 const char *sqlite3ErrStr(int rc){
  static const char* const aMsg[] = {
    /* SQLITE_OK          */ "not an error",
    /* SQLITE_ERROR       */ "SQL logic error or missing database",
    /* SQLITE_INTERNAL    */ 0,
    /* SQLITE_PERM        */ "access permission denied",
    /* SQLITE_ABORT       */ "callback requested query abort",
    /* SQLITE_BUSY        */ "database is locked",
    /* SQLITE_LOCKED      */ "database table is locked",
    /* SQLITE_NOMEM       */ "out of memory",
    /* SQLITE_READONLY    */ "attempt to write a readonly database",
    /* SQLITE_INTERRUPT   */ "interrupted",
    /* SQLITE_IOERR       */ "disk I/O error",
    /* SQLITE_CORRUPT     */ "database disk image is malformed",
    /* SQLITE_NOTFOUND    */ "unknown operation",
    /* SQLITE_FULL        */ "database or disk is full",
    /* SQLITE_CANTOPEN    */ "unable to open database file",
    /* SQLITE_PROTOCOL    */ "locking protocol",
    /* SQLITE_EMPTY       */ "table contains no data",
    /* SQLITE_SCHEMA      */ "database schema has changed",
    /* SQLITE_TOOBIG      */ "string or blob too big",
    /* SQLITE_CONSTRAINT  */ "constraint failed",
    /* SQLITE_MISMATCH    */ "datatype mismatch",
    /* SQLITE_MISUSE      */ "library routine called out of sequence",
    /* SQLITE_NOLFS       */ "large file support is disabled",
    /* SQLITE_AUTH        */ "authorization denied",
    /* SQLITE_FORMAT      */ "auxiliary database format error",
    /* SQLITE_RANGE       */ "bind or column index out of range",
    /* SQLITE_NOTADB      */ "file is encrypted or is not a database",
  };
  const char *zErr = "unknown error";
  switch( rc ){
    case SQLITE_ABORT_ROLLBACK: {
      zErr = "abort due to ROLLBACK";
      break;
    }
    default: {
      rc &= 0xff;
      if( rc>=0 && rc<ArraySize(aMsg) && aMsg[rc]!=0 ){
        zErr = aMsg[rc];
      }
      break;
    }
  }
  return zErr;
}

/*
** This routine implements a busy callback that sleeps and tries
** again until a timeout value is reached.  The timeout value is
** an integer number of milliseconds passed in as the first
** argument.
*/
static int sqliteDefaultBusyCallback(
 void *ptr,               /* Database connection */
 int count                /* Number of times table has been busy */
){
#if defined(HAVE_USLEEP) && HAVE_USLEEP
  static const byte delays[] =
     { 1, 2, 5, 10, 15, 20, 25, 25,  25,  50,  50, 100 };
  static const byte totals[] =
     { 0, 1, 3,  8, 18, 33, 53, 78, 103, 128, 178, 228 };
# define NDELAY ArraySize(delays)
  sqlite3 *db = (sqlite3 *)ptr;
  int timeout = db.busyTimeout;
  int delay, prior;

  assert( count>=0 );
  if( count < NDELAY ){
    delay = delays[count];
    prior = totals[count];
  }else{
    delay = delays[NDELAY-1];
    prior = totals[NDELAY-1] + delay*(count-(NDELAY-1));
  }
  if( prior + delay > timeout ){
    delay = timeout - prior;
    if( delay<=0 ) return 0;
  }
  sqlite3OsSleep(db.pVfs, delay*1000);
  return 1;
#else
  sqlite3 *db = (sqlite3 *)ptr;
  int timeout = ((sqlite3 *)ptr).busyTimeout;
  if( (count+1)*1000 > timeout ){
    return 0;
  }
  sqlite3OsSleep(db.pVfs, 1000000);
  return 1;
#endif
}

/*
** Invoke the given busy handler.
**
** This routine is called when an operation failed with a lock.
** If this routine returns non-zero, the lock is retried.  If it
** returns 0, the operation aborts with an SQLITE_BUSY error.
*/
 int sqlite3InvokeBusyHandler(BusyHandler *p){
  int rc;
  if( p==0 || p.xFunc==0 || p.nBusy<0 ) return 0;
  rc = p.xFunc(p.pArg, p.nBusy);
  if( rc==0 ){
    p.nBusy = -1;
  }else{
    p.nBusy++;
  }
  return rc; 
}

/*
** This routine sets the busy callback for an Sqlite database to the
** given callback function with the given argument.
*/
func int sqlite3_busy_handler(
  sqlite3 *db,
  int (*xBusy)(void*,int),
  void *pArg
){
  db.mutex.Lock()
  db.busyHandler.xFunc = xBusy;
  db.busyHandler.pArg = pArg;
  db.busyHandler.nBusy = 0;
  db.mutex.Unlock()
  return SQLITE_OK;
}

#ifndef SQLITE_OMIT_PROGRESS_CALLBACK
//	This routine sets the progress callback for an Sqlite database to the given callback function with the given argument. The progress callback will be invoked every nOps opcodes.
func (db *sqlite3) ProgressHandler(interval int, callback func(interface{}) int, arg interface{}) {
	db.mutex.Lock()
	if interval > 0 {
		db.xProgress = callback
		db.nProgressOps = interval
		db.pProgressArg = arg
	} else {
		db.xProgress = nil
		db.nProgressOps = 0
		db.pProgressArg = nil
	}
	db.mutex.Unlock()
}
#endif


/*
** This routine installs a default busy handler that waits for the
** specified number of milliseconds before returning 0.
*/
func int sqlite3_busy_timeout(sqlite3 *db, int ms){
  if( ms>0 ){
    db.busyTimeout = ms;
    sqlite3_busy_handler(db, sqliteDefaultBusyCallback, (void*)db);
  }else{
    sqlite3_busy_handler(db, 0, 0);
  }
  return SQLITE_OK;
}

/*
** Cause any pending operation to stop at its earliest opportunity.
*/
func void sqlite3_interrupt(sqlite3 *db){
  db.u1.isInterrupted = 1;
}


/*
** This function is exactly the same as sqlite3_create_function(), except
** that it is designed to be called by internal code. The difference is
** that if a malloc() fails in sqlite3_create_function(), an error code
** is returned and the mallocFailed flag cleared. 
*/
 int sqlite3CreateFunc(
  sqlite3 *db,
  const char *zFunctionName,
  int nArg,
  int enc,
  void *pUserData,
  void (*xFunc)(sqlite3_context*,int,sqlite3_value **),
  void (*xStep)(sqlite3_context*,int,sqlite3_value **),
  void (*xFinal)(sqlite3_context*),
  FuncDestructor *pDestructor
){
  FuncDef *p;
  int nName;

  if( zFunctionName==0 ||
      (xFunc && (xFinal || xStep)) || 
      (!xFunc && (xFinal && !xStep)) ||
      (!xFunc && (!xFinal && xStep)) ||
      (nArg<-1 || nArg>SQLITE_MAX_FUNCTION_ARG) ||
      (255<(nName = sqlite3Strlen30( zFunctionName))) ){
    return SQLITE_MISUSE_BKPT;
  }
  
  enc = SQLITE_UTF8;

  /* Check if an existing function is being overridden or deleted. If so,
  ** and there are active VMs, then return SQLITE_BUSY. If a function
  ** is being overridden/deleted but there are no active VMs, allow the
  ** operation to continue but invalidate all precompiled statements.
  */
  p = db.FindFunction(zFunctionName, nArg, byte(enc), false)
  if( p && p.iPrefEnc==enc && p.nArg==nArg ){
    if( db.activeVdbeCnt ){
      db.Error(SQLITE_BUSY, "unable to delete/modify user-function due to active statements");
      assert( !db.mallocFailed );
      return SQLITE_BUSY;
    }else{
      db.ExpirePreparedStatements()
    }
  }

  p = db.FindFunction(zFunctionName, nArg, byte(enc), true)
  assert(p || db.mallocFailed);
  if( !p ){
    return SQLITE_NOMEM;
  }

  /* If an older version of the function with a configured destructor is
  ** being replaced invoke the destructor function here. */
  functionDestroy(db, p);

  if( pDestructor ){
    pDestructor.nRef++;
  }
  p.pDestructor = pDestructor;
  p.flags = 0;
  p.xFunc = xFunc;
  p.xStep = xStep;
  p.xFinalize = xFinal;
  p.pUserData = pUserData;
  p.nArg = (uint16)nArg;
  return SQLITE_OK;
}

/*
** Create new user functions.
*/
func int sqlite3_create_function(
  sqlite3 *db,
  const char *zFunc,
  int nArg,
  int enc,
  void *p,
  void (*xFunc)(sqlite3_context*,int,sqlite3_value **),
  void (*xStep)(sqlite3_context*,int,sqlite3_value **),
  void (*xFinal)(sqlite3_context*)
){
  return sqlite3_create_function_v2(db, zFunc, nArg, enc, p, xFunc, xStep,
                                    xFinal, 0);
}

func int sqlite3_create_function_v2(
  sqlite3 *db,
  const char *zFunc,
  int nArg,
  int enc,
  void *p,
  void (*xFunc)(sqlite3_context*,int,sqlite3_value **),
  void (*xStep)(sqlite3_context*,int,sqlite3_value **),
  void (*xFinal)(sqlite3_context*),
  void (*xDestroy)(void *)
){
  int rc = SQLITE_ERROR;
  FuncDestructor *pArg = 0;
  db.mutex.Lock()
  if( xDestroy ){
    pArg = (FuncDestructor *)sqlite3DbMallocZero(db, sizeof(FuncDestructor));
    if( !pArg ){
      xDestroy(p);
      goto out;
    }
    pArg.xDestroy = xDestroy;
    pArg.pUserData = p;
  }
  rc = sqlite3CreateFunc(db, zFunc, nArg, enc, p, xFunc, xStep, xFinal, pArg);
  if( pArg && pArg.nRef==0 ){
    assert( rc!=SQLITE_OK );
    xDestroy(p);
    pArg = nil
  }

 out:
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

/*
** Declare that a function has been overloaded by a virtual table.
**
** If the function already exists as a regular global function, then
** this routine is a no-op.  If the function does not exist, then create
** a new one that always throws a run-time error.  
**
** When virtual tables intend to provide an overloaded function, they
** should call this routine to make sure the global function exists.
** A global function must exist in order for name resolution to work
** properly.
*/
func int sqlite3_overload_function(
  sqlite3 *db,
  const char *Name,
  int nArg
){
  int nName = sqlite3Strlen30(Name);
  int rc = SQLITE_OK;
  db.mutex.Lock()
  if db.FindFunction(Name, nArg, SQLITE_UTF8, false) == nil {
    rc = sqlite3CreateFunc(db, Name, nArg, SQLITE_UTF8,
                           0, sqlite3InvalidFunction, 0, 0, 0);
  }
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

#ifndef SQLITE_OMIT_TRACE
/*
** Register a trace function.  The pArg from the previously registered trace
** is returned.  
**
** A NULL trace function means that no tracing is executes.  A non-NULL
** trace is a pointer to a function that is invoked at the start of each
** SQL statement.
*/
func void *sqlite3_trace(sqlite3 *db, void (*xTrace)(void*,const char*), void *pArg){
  void *pOld;
  db.mutex.Lock()
  pOld = db.pTraceArg;
  db.xTrace = xTrace;
  db.pTraceArg = pArg;
  db.mutex.Unlock()
  return pOld;
}

//	Register a profile function. The pArg from the previously registered profile function is returned.  
//
//	A NULL profile function means that no profiling is executes. A non-NULL profile is a pointer to a function that is invoked at the conclusion of
//	each SQL statement that is run.
func void *sqlite3_profile(sqlite3 *db, void (*xProfile)(void*,const char*,uint64), void *pArg){
	void *pOld;
	db.mutex.Lock()
	pOld = db.pProfileArg;
	db.xProfile = xProfile;
	db.pProfileArg = pArg;
	db.mutex.Unlock()
	return pOld;
}
#endif /* SQLITE_OMIT_TRACE */

/*
** Register a function to be invoked when a transaction commits.
** If the invoked function returns non-zero, then the commit becomes a
** rollback.
*/
func void *sqlite3_commit_hook(
  sqlite3 *db,              /* Attach the hook to this database */
  int (*xCallback)(void*),  /* Function to invoke on each commit */
  void *pArg                /* Argument to the function */
){
  void *pOld;
  db.mutex.Lock()
  pOld = db.pCommitArg;
  db.xCommitCallback = xCallback;
  db.pCommitArg = pArg;
  db.mutex.Unlock()
  return pOld;
}

/*
** Register a callback to be invoked each time a row is updated,
** inserted or deleted using this database connection.
*/
func void *sqlite3_update_hook(
  sqlite3 *db,              /* Attach the hook to this database */
  void (*xCallback)(void*,int,char const *,char const *,sqlite_int64),
  void *pArg                /* Argument to the function */
){
  void *pRet;
  db.mutex.Lock()
  pRet = db.pUpdateArg;
  db.xUpdateCallback = xCallback;
  db.pUpdateArg = pArg;
  db.mutex.Unlock()
  return pRet;
}

/*
** Register a callback to be invoked each time a transaction is rolled
** back by this database connection.
*/
func void *sqlite3_rollback_hook(
  sqlite3 *db,              /* Attach the hook to this database */
  void (*xCallback)(void*), /* Callback function */
  void *pArg                /* Argument to the function */
){
  void *pRet;
  db.mutex.Lock()
  pRet = db.pRollbackArg;
  db.xRollbackCallback = xCallback;
  db.pRollbackArg = pArg;
  db.mutex.Unlock()
  return pRet;
}

#ifndef SQLITE_OMIT_WAL
/*
** The sqlite3_wal_hook() callback registered by sqlite3_wal_autocheckpoint().
** Invoke sqlite3_wal_checkpoint if the number of frames in the log file
** is greater than sqlite3.pWalArg cast to an integer (the value configured by
** wal_autocheckpoint()).
*/ 
 int sqlite3WalDefaultHook(
  void *pClientData,     /* Argument */
  sqlite3 *db,           /* Connection */
  const char *zDb,       /* Database */
  int nFrame             /* Size of WAL */
){
  if( nFrame>=SQLITE_PTR_TO_INT(pClientData) ){
    sqlite3_wal_checkpoint(db, zDb);
  }
  return SQLITE_OK;
}
#endif /* SQLITE_OMIT_WAL */

/*
** Configure an sqlite3_wal_hook() callback to automatically checkpoint
** a database after committing a transaction if there are nFrame or
** more frames in the log file. Passing zero or a negative value as the
** nFrame parameter disables automatic checkpoints entirely.
**
** The callback registered by this function replaces any existing callback
** registered using sqlite3_wal_hook(). Likewise, registering a callback
** using sqlite3_wal_hook() disables the automatic checkpoint mechanism
** configured by this function.
*/
func int sqlite3_wal_autocheckpoint(sqlite3 *db, int nFrame){
#ifndef SQLITE_OMIT_WAL
  if( nFrame>0 ){
    sqlite3_wal_hook(db, sqlite3WalDefaultHook, SQLITE_INT_TO_PTR(nFrame));
  }else{
    sqlite3_wal_hook(db, 0, 0);
  }
#endif
  return SQLITE_OK;
}

/*
** Register a callback to be invoked each time a transaction is written
** into the write-ahead-log by this database connection.
*/
func void *sqlite3_wal_hook(
  sqlite3 *db,                    /* Attach the hook to this db handle */
  int(*xCallback)(void *, sqlite3*, const char*, int),
  void *pArg                      /* First argument passed to xCallback() */
){
#ifndef SQLITE_OMIT_WAL
  void *pRet;
  db.mutex.Lock()
  pRet = db.pWalArg;
  db.xWalCallback = xCallback;
  db.pWalArg = pArg;
  db.mutex.Unlock()
  return pRet;
#else
  return 0;
#endif
}

/*
** Checkpoint database zDb.
*/
func int sqlite3_wal_checkpoint_v2(
  sqlite3 *db,                    /* Database handle */
  const char *zDb,                /* Name of attached database (or NULL) */
  int eMode,                      /* SQLITE_CHECKPOINT_* value */
  int *pnLog,                     /* OUT: Size of WAL log in frames */
  int *pnCkpt                     /* OUT: Total number of frames checkpointed */
){
#ifdef SQLITE_OMIT_WAL
  return SQLITE_OK;
#else
  int rc;                         /* Return code */
  int iDb = SQLITE_MAX_ATTACHED;  /* sqlite3pSelect[] index of db to checkpoint */

  /* Initialize the output variables to -1 in case an error occurs. */
  if( pnLog ) *pnLog = -1;
  if( pnCkpt ) *pnCkpt = -1;

  assert( SQLITE_CHECKPOINT_FULL>SQLITE_CHECKPOINT_PASSIVE );
  assert( SQLITE_CHECKPOINT_FULL<SQLITE_CHECKPOINT_RESTART );
  assert( SQLITE_CHECKPOINT_PASSIVE+2==SQLITE_CHECKPOINT_RESTART );
  if( eMode<SQLITE_CHECKPOINT_PASSIVE || eMode>SQLITE_CHECKPOINT_RESTART ){
    return SQLITE_MISUSE;
  }

  db.mutex.Lock()
  if zDb != nil && zDb[0] {
    iDb = db.FindDbName(zDb)
  }
  if iDb < 0 {
    rc = SQLITE_ERROR;
    db.Error(SQLITE_ERROR, "unknown database: %v", zDb);
  }else{
    rc = sqlite3Checkpoint(db, iDb, eMode, pnLog, pnCkpt);
    db.Error(rc, "");
  }
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
#endif
}


/*
** Checkpoint database zDb. If zDb is NULL, or if the buffer zDb points
** to contains a zero-length string, all attached databases are 
** checkpointed.
*/
func int sqlite3_wal_checkpoint(sqlite3 *db, const char *zDb){
  return sqlite3_wal_checkpoint_v2(db, zDb, SQLITE_CHECKPOINT_PASSIVE, 0, 0);
}

#ifndef SQLITE_OMIT_WAL
/*
** Run a checkpoint on database iDb. This is a no-op if database iDb is
** not currently open in WAL mode.
**
** If a transaction is open on the database being checkpointed, this 
** function returns SQLITE_LOCKED and a checkpoint is not attempted. If 
** an error occurs while running the checkpoint, an SQLite error code is 
** returned (i.e. SQLITE_IOERR). Otherwise, SQLITE_OK.
**
** The mutex on database handle db should be held by the caller. The mutex
** associated with the specific b-tree being checkpointed is taken by
** this function while the checkpoint is running.
**
** If iDb is passed SQLITE_MAX_ATTACHED, then all attached databases are
** checkpointed. If an error is encountered it is returned immediately -
** no attempt is made to checkpoint any remaining databases.
**
** Parameter eMode is one of SQLITE_CHECKPOINT_PASSIVE, FULL or RESTART.
*/
 int sqlite3Checkpoint(sqlite3 *db, int iDb, int eMode, int *pnLog, int *pnCkpt){
  int rc = SQLITE_OK;             /* Return code */
  int i;                          /* Used to iterate through attached dbs */
  int bBusy = 0;                  /* True if SQLITE_BUSY has been encountered */

  assert( !pnLog || *pnLog==-1 );
  assert( !pnCkpt || *pnCkpt==-1 );

  for(i=0; i<db.nDb && rc==SQLITE_OK; i++){
    if( i==iDb || iDb==SQLITE_MAX_ATTACHED ){
      rc = sqlite3BtreeCheckpoint(db.Databases[i].pBt, eMode, pnLog, pnCkpt);
      pnLog = 0;
      pnCkpt = 0;
      if( rc==SQLITE_BUSY ){
        bBusy = 1;
        rc = SQLITE_OK;
      }
    }
  }

  return (rc==SQLITE_OK && bBusy) ? SQLITE_BUSY : rc;
}
#endif /* SQLITE_OMIT_WAL */

/*
** This function returns true if main-memory should be used instead of
** a temporary file for transient pager files and statement journals.
** The value returned depends on the value of db.temp_store (runtime
** parameter) and the compile time value of SQLITE_TEMP_STORE. The
** following table describes the relationship between these two values
** and this functions return value.
**
**   SQLITE_TEMP_STORE     db.temp_store     Location of temporary database
**   -----------------     --------------     ------------------------------
**   0                     any                file      (return 0)
**   1                     1                  file      (return 0)
**   1                     2                  memory    (return 1)
**   1                     0                  file      (return 0)
**   2                     1                  file      (return 0)
**   2                     2                  memory    (return 1)
**   2                     0                  memory    (return 1)
**   3                     any                memory    (return 1)
*/
 int sqlite3TempInMemory(const sqlite3 *db){
#if SQLITE_TEMP_STORE==1
  return ( db.temp_store==2 );
#endif
#if SQLITE_TEMP_STORE==2
  return ( db.temp_store!=1 );
#endif
#if SQLITE_TEMP_STORE==3
  return 1;
#endif
#if SQLITE_TEMP_STORE<1 || SQLITE_TEMP_STORE>3
  return 0;
#endif
}

/*
** Return UTF-8 encoded English language explanation of the most recent
** error.
*/
func const char *sqlite3_errmsg(sqlite3 *db){
  const char *z;
  if( !db ){
    return sqlite3ErrStr(SQLITE_NOMEM);
  }
  if !db.SafetyCheckSickOrOk() {
    return sqlite3ErrStr(SQLITE_MISUSE_BKPT);
  }
  db.mutex.Lock()
  if( db.mallocFailed ){
    z = sqlite3ErrStr(SQLITE_NOMEM);
  }else{
    z = (char*)sqlite3_value_text(db.pErr);
    assert( !db.mallocFailed );
    if( z==0 ){
      z = sqlite3ErrStr(db.errCode);
    }
  }
  db.mutex.Unlock()
  return z;
}

/*
** Return the most recent error code generated by an SQLite routine. If NULL is
** passed to this function, we assume a malloc() failed during sqlite3_open().
*/
func int sqlite3_errcode(sqlite3 *db){
  if db != nil && !db.SafetyCheckSickOrOk() {
    return SQLITE_MISUSE_BKPT;
  }
  if db == nil || db.mallocFailed {
    return SQLITE_NOMEM;
  }
  return db.errCode & db.errMask;
}
func int sqlite3_extended_errcode(sqlite3 *db){
  if db != nil && !db.SafetyCheckSickOrOk() {
    return SQLITE_MISUSE_BKPT;
  }
  if db == nil || db.mallocFailed {
    return SQLITE_NOMEM;
  }
  return db.errCode;
}

/*
** Create a new collating function for database "db".  The name is Name
** and the encoding is enc.
*/
static int createCollation(
  sqlite3* db,
  const char *Name, 
  byte enc,
  void* pCtx,
  int(*xCompare)(void*,int,const void*,int,const void*),
  void(*xDel)(void*)
){
  CollSeq *pColl;
  int enc2;
  int nName = sqlite3Strlen30(Name);
  
  enc2 = enc;
  if enc2 != SQLITE_UTF8 {
    return SQLITE_MISUSE_BKPT
  }

  /* Check if this call is removing or replacing an existing collation 
  ** sequence. If so, and there are active VMs, return busy. If there
  ** are no active VMs, invalidate any pre-compiled statements.
  */
  pColl = sqlite3FindCollSeq(db, (byte)enc2, Name, 0);
  if( pColl && pColl.xCmp ){
    if( db.activeVdbeCnt ){
      db.Error(SQLITE_BUSY, "unable to delete/modify collation sequence due to active statements");
      return SQLITE_BUSY;
    }
    db.ExpirePreparedStatements()

    /* If collation sequence pColl was created directly by a call to
    ** sqlite3_create_collation, and not generated by synthCollSeq(),
    ** then any copies made by synthCollSeq() need to be invalidated.
    ** Also, collation destructor - CollSeq.xDel() - function may need
    ** to be called.
    */ 
    if pColl.enc == enc2 {
      CollSeq *aColl = db.Collations[Name]
      int j;
      for(j=0; j<3; j++){
        CollSeq *p = &aColl[j];
        if( p.enc==pColl.enc ){
          if( p.xDel ){
            p.xDel(p.pUser);
          }
          p.xCmp = 0;
        }
      }
    }
  }

  pColl = sqlite3FindCollSeq(db, (byte)enc2, Name, 1);
  if( pColl==0 ) return SQLITE_NOMEM;
  pColl.xCmp = xCompare;
  pColl.pUser = pCtx;
  pColl.xDel = xDel;
  pColl.enc = byte(enc2)
  db.Error(SQLITE_OK, "");
  return SQLITE_OK;
}


/*
** This array defines hard upper bounds on limit values.  The
** initializer must be kept in sync with the SQLITE_LIMIT_*
** #defines in sqlite3.h.
*/
static const int aHardLimit[] = {
  SQLITE_MAX_LENGTH,
  SQLITE_MAX_SQL_LENGTH,
  SQLITE_MAX_COMPOUND_SELECT,
  SQLITE_MAX_VDBE_OP,
  SQLITE_MAX_FUNCTION_ARG,
  SQLITE_MAX_ATTACHED,
  SQLITE_MAX_LIKE_PATTERN_LENGTH,
  SQLITE_MAX_TRIGGER_DEPTH,
};

/*
** Make sure the hard limits are set to reasonable values
*/
#if SQLITE_MAX_LENGTH<100
# error SQLITE_MAX_LENGTH must be at least 100
#endif
#if SQLITE_MAX_SQL_LENGTH<100
# error SQLITE_MAX_SQL_LENGTH must be at least 100
#endif
#if SQLITE_MAX_SQL_LENGTH>SQLITE_MAX_LENGTH
# error SQLITE_MAX_SQL_LENGTH must not be greater than SQLITE_MAX_LENGTH
#endif
#if SQLITE_MAX_COMPOUND_SELECT<2
# error SQLITE_MAX_COMPOUND_SELECT must be at least 2
#endif
#if SQLITE_MAX_VDBE_OP<40
# error SQLITE_MAX_VDBE_OP must be at least 40
#endif
#if SQLITE_MAX_FUNCTION_ARG<0 || SQLITE_MAX_FUNCTION_ARG>1000
# error SQLITE_MAX_FUNCTION_ARG must be between 0 and 1000
#endif
#if SQLITE_MAX_ATTACHED<0 || SQLITE_MAX_ATTACHED>62
# error SQLITE_MAX_ATTACHED must be between 0 and 62
#endif
#if SQLITE_MAX_LIKE_PATTERN_LENGTH<1
# error SQLITE_MAX_LIKE_PATTERN_LENGTH must be at least 1
#endif

#if SQLITE_MAX_TRIGGER_DEPTH<1
# error SQLITE_MAX_TRIGGER_DEPTH must be at least 1
#endif


/*
** Change the value of a limit.  Report the old value.
** If an invalid limit index is supplied, report -1.
** Make no changes but still report the old value if the
** new limit is negative.
**
** A new lower limit does not shrink existing constructs.
** It merely prevents new constructs that exceed the limit
** from forming.
*/
func int sqlite3_limit(sqlite3 *db, int limitId, int newLimit){
  int oldLimit;


  /* EVIDENCE-OF: R-30189-54097 For each limit category SQLITE_LIMIT_NAME
  ** there is a hard upper bound set at compile-time by a C preprocessor
  ** macro called SQLITE_MAX_NAME. (The "_LIMIT_" in the name is changed to
  ** "_MAX_".)
  */
  assert( aHardLimit[SQLITE_LIMIT_LENGTH]==SQLITE_MAX_LENGTH );
  assert( aHardLimit[SQLITE_LIMIT_SQL_LENGTH]==SQLITE_MAX_SQL_LENGTH );
  assert( aHardLimit[SQLITE_LIMIT_COMPOUND_SELECT]==SQLITE_MAX_COMPOUND_SELECT);
  assert( aHardLimit[SQLITE_LIMIT_VDBE_OP]==SQLITE_MAX_VDBE_OP );
  assert( aHardLimit[SQLITE_LIMIT_FUNCTION_ARG]==SQLITE_MAX_FUNCTION_ARG );
  assert( aHardLimit[SQLITE_LIMIT_ATTACHED]==SQLITE_MAX_ATTACHED );
  assert( aHardLimit[SQLITE_LIMIT_LIKE_PATTERN_LENGTH]==
                                               SQLITE_MAX_LIKE_PATTERN_LENGTH );
  assert( aHardLimit[SQLITE_LIMIT_TRIGGER_DEPTH]==SQLITE_MAX_TRIGGER_DEPTH );
  assert( SQLITE_LIMIT_TRIGGER_DEPTH==(SQLITE_N_LIMIT-1) );


  if( limitId<0 || limitId>=SQLITE_N_LIMIT ){
    return -1;
  }
  oldLimit = db.aLimit[limitId];
  if( newLimit>=0 ){                   /* IMP: R-52476-28732 */
    if( newLimit>aHardLimit[limitId] ){
      newLimit = aHardLimit[limitId];  /* IMP: R-51463-25634 */
    }
    db.aLimit[limitId] = newLimit;
  }
  return oldLimit;                     /* IMP: R-53341-35419 */
}

/*
** This function is used to parse both URIs and non-URI filenames passed by the
** user to API functions sqlite3_open() or sqlite3_open_v2(), and for database
** URIs specified as part of ATTACH statements.
**
** The first argument to this function is the name of the VFS to use (or
** a NULL to signify the default VFS) if the URI does not contain a "vfs=xxx"
** query parameter. The second argument contains the URI (or non-URI filename)
** itself. When this function is called the *pFlags variable should contain
** the default flags to open the database handle with. The value stored in
** *pFlags may be updated before returning if the URI filename contains 
** "cache=xxx" or "mode=xxx" query parameters.
**
** If successful, SQLITE_OK is returned. In this case *ppVfs is set to point to
** the VFS that should be used to open the database file. *pzFile is set to
** point to a buffer containing the name of the file to open.
**
** If an error occurs, then an SQLite error code is returned and *pzErrMsg
** may be set to point to a buffer containing an English language error 
** message.
*/
 int sqlite3ParseUri(
  const char *zDefaultVfs,        /* VFS to use if no "vfs=xxx" query option */
  const char *zUri,               /* Nul-terminated URI to parse */
  uint *pFlags,           /* IN/OUT: SQLITE_OPEN_XXX flags */
  sqlite3_vfs **ppVfs,            /* OUT: VFS to use */ 
  char **pzFile,                  /* OUT: Filename component of URI */
  char **pzErrMsg                 /* OUT: Error message (if rc!=SQLITE_OK) */
){
  int rc = SQLITE_OK;
  uint flags = *pFlags;
  const char *zVfs = zDefaultVfs;
  char *zFile;
  char c;
  int nUri = sqlite3Strlen30(zUri);

  assert( *pzErrMsg==0 );

  if( ((flags & SQLITE_OPEN_URI) || sqlite3GlobalConfig.bOpenUri) 
   && nUri>=5 && memcmp(zUri, "file:", 5)==0 
  ){
    char *zOpt;
    int eState;                   /* Parser state when parsing URI */
    int iIn;                      /* Input character index */
    int iOut = 0;                 /* Output character index */
    int nByte = nUri+2;           /* Bytes of space to allocate */

    /* Make sure the SQLITE_OPEN_URI flag is set to indicate to the VFS xOpen 
    ** method that there may be extra parameters following the file-name.  */
    flags |= SQLITE_OPEN_URI;

    for(iIn=0; iIn<nUri; iIn++) nByte += (zUri[iIn]=='&');
    zFile = sqlite3_malloc(nByte);
    if( !zFile ) return SQLITE_NOMEM;

    /* Discard the scheme and authority segments of the URI. */
    if( zUri[5]=='/' && zUri[6]=='/' ){
      iIn = 7;
      while( zUri[iIn] && zUri[iIn]!='/' ) iIn++;

      if( iIn!=7 && (iIn!=16 || memcmp("localhost", &zUri[7], 9)) ){
        *pzErrMsg = fmt.Sprintf("invalid uri authority: %v*%v", iIn-7, &zUri[7]);
        rc = SQLITE_ERROR;
        goto parse_uri_out;
      }
    }else{
      iIn = 5;
    }

    /* Copy the filename and any query parameters into the zFile buffer. 
    ** Decode %HH escape codes along the way. 
    **
    ** Within this loop, variable eState may be set to 0, 1 or 2, depending
    ** on the parsing context. As follows:
    **
    **   0: Parsing file-name.
    **   1: Parsing name section of a name=value query parameter.
    **   2: Parsing value section of a name=value query parameter.
    */
    eState = 0;
    while( (c = zUri[iIn])!=0 && c!='#' ){
      iIn++;
      if( c=='%' 
       && sqlite3Isxdigit(zUri[iIn]) 
       && sqlite3Isxdigit(zUri[iIn+1]) 
      ){
        int octet = (sqlite3HexToInt(zUri[iIn++]) << 4);
        octet += sqlite3HexToInt(zUri[iIn++]);

        assert( octet>=0 && octet<256 );
        if( octet==0 ){
          /* This branch is taken when "%00" appears within the URI. In this
          ** case we ignore all text in the remainder of the path, name or
          ** value currently being parsed. So ignore the current character
          ** and skip to the next "?", "=" or "&", as appropriate. */
          while( (c = zUri[iIn])!=0 && c!='#' 
              && (eState!=0 || c!='?')
              && (eState!=1 || (c!='=' && c!='&'))
              && (eState!=2 || c!='&')
          ){
            iIn++;
          }
          continue;
        }
        c = octet;
      }else if( eState==1 && (c=='&' || c=='=') ){
        if( zFile[iOut-1]==0 ){
          /* An empty option name. Ignore this option altogether. */
          while( zUri[iIn] && zUri[iIn]!='#' && zUri[iIn-1]!='&' ) iIn++;
          continue;
        }
        if( c=='&' ){
          zFile[iOut++] = '\0';
        }else{
          eState = 2;
        }
        c = 0;
      }else if( (eState==0 && c=='?') || (eState==2 && c=='&') ){
        c = 0;
        eState = 1;
      }
      zFile[iOut++] = c;
    }
    if( eState==1 ) zFile[iOut++] = '\0';
    zFile[iOut++] = '\0';
    zFile[iOut++] = '\0';

    /* Check if there were any options specified that should be interpreted 
    ** here. Options that are interpreted here include "vfs" and those that
    ** correspond to flags that may be passed to the sqlite3_open_v2()
    ** method. */
    zOpt = &zFile[sqlite3Strlen30(zFile)+1];
    while( zOpt[0] ){
      int nOpt = sqlite3Strlen30(zOpt);
      char *zVal = &zOpt[nOpt+1];
      int nVal = sqlite3Strlen30(zVal);

      if( nOpt==3 && memcmp("vfs", zOpt, 3)==0 ){
        zVfs = zVal;
      }else{
        struct OpenMode {
          const char *z;
          int mode;
        } *aMode = 0;
        char *zModeType = 0;
        int mask = 0;
        int limit = 0;

        if( nOpt==5 && memcmp("cache", zOpt, 5)==0 ){
          static struct OpenMode aCacheMode[] = {
            { "shared",  SQLITE_OPEN_SHAREDCACHE },
            { "private", SQLITE_OPEN_PRIVATECACHE },
            { 0, 0 }
          };

          mask = SQLITE_OPEN_SHAREDCACHE|SQLITE_OPEN_PRIVATECACHE;
          aMode = aCacheMode;
          limit = mask;
          zModeType = "cache";
        }
        if( nOpt==4 && memcmp("mode", zOpt, 4)==0 ){
          static struct OpenMode aOpenMode[] = {
            { "ro",  SQLITE_OPEN_READONLY },
            { "rw",  SQLITE_OPEN_READWRITE }, 
            { "rwc", SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE },
            { 0, 0 }
          };

          mask = SQLITE_OPEN_READONLY|SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE;
          aMode = aOpenMode;
          limit = mask & flags;
          zModeType = "access";
        }

        if( aMode ){
          int i;
          int mode = 0;
          for(i=0; aMode[i].z; i++){
            const char *z = aMode[i].z;
            if( nVal==sqlite3Strlen30(z) && 0==memcmp(zVal, z, nVal) ){
              mode = aMode[i].mode;
              break;
            }
          }
          if( mode==0 ){
            *pzErrMsg = fmt.Sprintf("no such %v mode: %v", zModeType, zVal);
            rc = SQLITE_ERROR;
            goto parse_uri_out;
          }
          if( mode>limit ){
            *pzErrMsg = fmt.Sprintf("%v mode not allowed: %v", zModeType, zVal);
            rc = SQLITE_PERM;
            goto parse_uri_out;
          }
          flags = (flags & ~mask) | mode;
        }
      }

      zOpt = &zVal[nVal+1];
    }

  }else{
    zFile = sqlite3_malloc(nUri+2);
    if( !zFile ) return SQLITE_NOMEM;
    memcpy(zFile, zUri, nUri);
    zFile[nUri] = '\0';
    zFile[nUri+1] = '\0';
  }

  *ppVfs = sqlite3_vfs_find(zVfs);
  if( *ppVfs==0 ){
    *pzErrMsg = fmt.Sprintf("no such vfs: %v", zVfs);
    rc = SQLITE_ERROR;
  }
 parse_uri_out:
  if( rc!=SQLITE_OK ){
    zFile = nil
  }
  *pFlags = flags;
  *pzFile = zFile;
  return rc;
}


/*
** This routine does the work of opening a database on behalf of sqlite3_open(). The database filename "zFilename" is UTF-8 encoded.
*/
static int openDatabase(
  const char *zFilename, /* Database filename UTF-8 encoded */
  sqlite3 **ppDb,        /* OUT: Returned database handle */
  uint flags,    /* Operational flags */
  const char *zVfs       /* Name of the VFS to use */
){
  sqlite3 *db;                    /* Store allocated handle here */
  int rc;                         /* Return code */
  int isThreadsafe;               /* True for threadsafe connections */
  char *zOpen = 0;                /* Filename argument to pass to BtreeOpen() */
  char *zErrMsg = 0;              /* Error message from sqlite3ParseUri() */

  *ppDb = 0;

  /* Only allow sensible combinations of bits in the flags argument.  
  ** Throw an error if any non-sense combination is used.  If we
  ** do not block illegal combinations here, it could trigger
  ** assert() statements in deeper layers.  Sensible combinations
  ** are:
  **
  **  1:  SQLITE_OPEN_READONLY
  **  2:  SQLITE_OPEN_READWRITE
  **  6:  SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE
  */
  assert( SQLITE_OPEN_READONLY  == 0x01 );
  assert( SQLITE_OPEN_READWRITE == 0x02 );
  assert( SQLITE_OPEN_CREATE    == 0x04 );
  if( ((1<<(flags&7)) & 0x46)==0 ) return SQLITE_MISUSE_BKPT;

  if( sqlite3GlobalConfig.bCoreMutex==0 ){
    isThreadsafe = 0;
  }else if( flags & SQLITE_OPEN_NOMUTEX ){
    isThreadsafe = 0;
  }else if( flags & SQLITE_OPEN_FULLMUTEX ){
    isThreadsafe = 1;
  }else{
    isThreadsafe = sqlite3GlobalConfig.bFullMutex;
  }
  if( flags & SQLITE_OPEN_PRIVATECACHE ){
    flags &= ~SQLITE_OPEN_SHAREDCACHE;
  }else if( sqlite3GlobalConfig.sharedCacheEnabled ){
    flags |= SQLITE_OPEN_SHAREDCACHE;
  }

  /* Remove harmful bits from the flags parameter
  **
  ** The SQLITE_OPEN_NOMUTEX and SQLITE_OPEN_FULLMUTEX flags were
  ** dealt with in the previous code block.  Besides these, the only
  ** valid input flags for sqlite3_open_v2() are SQLITE_OPEN_READONLY,
  ** SQLITE_OPEN_READWRITE, SQLITE_OPEN_CREATE, SQLITE_OPEN_SHAREDCACHE,
  ** SQLITE_OPEN_PRIVATECACHE, and some reserved bits.  Silently mask
  ** off all other flags.
  */
  flags &=  ~( SQLITE_OPEN_DELETEONCLOSE |
               SQLITE_OPEN_EXCLUSIVE |
               SQLITE_OPEN_MAIN_DB |
               SQLITE_OPEN_TEMP_DB | 
               SQLITE_OPEN_TRANSIENT_DB | 
               SQLITE_OPEN_MAIN_JOURNAL | 
               SQLITE_OPEN_TEMP_JOURNAL | 
               SQLITE_OPEN_SUBJOURNAL | 
               SQLITE_OPEN_MASTER_JOURNAL |
               SQLITE_OPEN_NOMUTEX |
               SQLITE_OPEN_FULLMUTEX |
               SQLITE_OPEN_WAL
             );

  /* Allocate the sqlite data structure */
  db = sqlite3MallocZero( sizeof(sqlite3) );
  if( db==0 ) goto opendb_out;
  if( isThreadsafe ){
    db.mutex = sqlite3MutexAlloc(SQLITE_MUTEX_RECURSIVE);
    if( db.mutex==0 ){
      db = nil
      goto opendb_out;
    }
  }
  db.mutex.Lock()
  db.errMask = 0xff;
  db.nDb = 2;
  db.magic = SQLITE_MAGIC_BUSY;
  db.Databases = db.DatabasesStatic;

  assert( sizeof(db.aLimit)==sizeof(aHardLimit) );
  memcpy(db.aLimit, aHardLimit, sizeof(db.aLimit));
  db.autoCommit = 1;
  db.nextAutovac = -1;
  db.nextPagesize = 0;
  db.flags |= SQLITE_ShortColNames | SQLITE_AutoIndex | SQLITE_EnableTrigger
#if SQLITE_DEFAULT_FILE_FORMAT<4
                 | SQLITE_LegacyFileFmt
#endif
#ifdef SQLITE_ENABLE_LOAD_EXTENSION
                 | SQLITE_LoadExtension
#endif
#if SQLITE_DEFAULT_RECURSIVE_TRIGGERS
                 | SQLITE_RecTriggers
#endif
#if defined(SQLITE_DEFAULT_FOREIGN_KEYS) && SQLITE_DEFAULT_FOREIGN_KEYS
                 | SQLITE_ForeignKeys
#endif
      ;
  db.Collations := make(map[string]*CollSeq)
  db.Modules := make(map[string]*Module)

  /* Add the default collation sequence BINARY. The only error that can occur here is a malloc() failure. */
  createCollation(db, "BINARY", SQLITE_UTF8, 0, binCollFunc, 0);
  createCollation(db, "RTRIM", SQLITE_UTF8, (void*)1, binCollFunc, 0);
  if( db.mallocFailed ){
    goto opendb_out;
  }
  db.pDfltColl = sqlite3FindCollSeq(db, SQLITE_UTF8, "BINARY", 0);
  assert( db.pDfltColl!=0 );

  /* Also add a UTF-8 case-insensitive collation sequence. */
  createCollation(db, "NOCASE", SQLITE_UTF8, 0, nocaseCollatingFunc, 0);

  /* Parse the filename/URI argument. */
  db.openFlags = flags;
  rc = sqlite3ParseUri(zVfs, zFilename, &flags, &db.pVfs, &zOpen, &zErrMsg);
  if( rc!=SQLITE_OK ){
    if( rc==SQLITE_NOMEM ) db.mallocFailed = true
    db.Error(rc, zErrMsg ? "%v" : "", zErrMsg);
    zErrMsg = nil
    goto opendb_out;
  }

  /* Open the backend database driver */
  rc = sqlite3BtreeOpen(db.pVfs, zOpen, db, &db.Databases[0].pBt, 0,
                        flags | SQLITE_OPEN_MAIN_DB);
  if( rc!=SQLITE_OK ){
    if( rc==SQLITE_IOERR_NOMEM ){
      rc = SQLITE_NOMEM;
    }
    db.Error(rc, "");
    goto opendb_out;
  }
  db.Databases[0].Schema = db.GetSchema(db.Databases[0].pBt)
  db.Databases[1].Schema = db.GetSchema(nil)


  /* The default safety_level for the main database is 'full'; for the temp
  ** database it is 'NONE'. This matches the pager layer defaults.  
  */
  db.Databases[0].Name = "main";
  db.Databases[0].safety_level = 3;
  db.Databases[1].Name = "temp";
  db.Databases[1].safety_level = 1;

  db.magic = SQLITE_MAGIC_OPEN;
  if( db.mallocFailed ){
    goto opendb_out;
  }

  /* Register all built-in functions, but do not attempt to read the
  ** database schema yet. This is delayed until the first time the database
  ** is accessed.
  */
  db.Error(SQLITE_OK, "");
  sqlite3RegisterBuiltinFunctions(db);

  /* Load automatic extensions - extensions that have been registered
  ** using the sqlite3_automatic_extension() API.
  */
  rc = sqlite3_errcode(db);
  if( rc==SQLITE_OK ){
    sqlite3AutoLoadExtensions(db);
    rc = sqlite3_errcode(db);
    if( rc!=SQLITE_OK ){
      goto opendb_out;
    }
  }

#ifdef SQLITE_ENABLE_FTS1
  if( !db.mallocFailed ){
    extern int sqlite3Fts1Init(sqlite3*);
    rc = sqlite3Fts1Init(db);
  }
#endif

#ifdef SQLITE_ENABLE_FTS2
  if( !db.mallocFailed && rc==SQLITE_OK ){
    extern int sqlite3Fts2Init(sqlite3*);
    rc = sqlite3Fts2Init(db);
  }
#endif

#ifdef SQLITE_ENABLE_FTS3
  if( !db.mallocFailed && rc==SQLITE_OK ){
    rc = sqlite3Fts3Init(db);
  }
#endif

#ifdef SQLITE_ENABLE_RTREE
  if( !db.mallocFailed && rc==SQLITE_OK){
    rc = sqlite3RtreeInit(db);
  }
#endif

  db.Error(rc, "");

  /* -DSQLITE_DEFAULT_LOCKING_MODE=1 makes EXCLUSIVE the default locking
  ** mode.  -DSQLITE_DEFAULT_LOCKING_MODE=0 make NORMAL the default locking
  ** mode.  Doing nothing at all also makes NORMAL the default.
  */
#ifdef SQLITE_DEFAULT_LOCKING_MODE
  db.dfltLockMode = SQLITE_DEFAULT_LOCKING_MODE;
  sqlite3PagerLockingMode(db.Databases[0].pBt.Pager(),
                          SQLITE_DEFAULT_LOCKING_MODE);
#endif

  /* Enable the lookaside-malloc subsystem */
  setupLookaside(db, 0, sqlite3GlobalConfig.szLookaside,
                        sqlite3GlobalConfig.nLookaside);

  sqlite3_wal_autocheckpoint(db, SQLITE_DEFAULT_WAL_AUTOCHECKPOINT);

opendb_out:
  zOpen = nil
  if( db ){
    assert( db.mutex!=0 || isThreadsafe==0 || sqlite3GlobalConfig.bFullMutex==0 );
    db.mutex.Unlock()
  }
  rc = sqlite3_errcode(db);
  assert( db!=0 || rc==SQLITE_NOMEM );
  if( rc==SQLITE_NOMEM ){
    db.Close()
    db = nil
  }else if( rc!=SQLITE_OK ){
    db.magic = SQLITE_MAGIC_SICK;
  }
  *ppDb = db;
  return (*sqlite3)(nil).ApiExit(rc)
}

/*
** Open a new database handle.
*/
func int sqlite3_open(
  const char *zFilename, 
  sqlite3 **ppDb 
){
  return openDatabase(zFilename, ppDb,
                      SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE, 0);
}
func int sqlite3_open_v2(
  const char *filename,   /* Database filename (UTF-8) */
  sqlite3 **ppDb,         /* OUT: SQLite db handle */
  int flags,              /* Flags */
  const char *zVfs        /* Name of VFS module to use */
){
  return openDatabase(filename, ppDb, (uint)flags, zVfs);
}

/*
** Register a new collation sequence with the database handle db.
*/
func int sqlite3_create_collation(
  sqlite3* db, 
  const char *Name, 
  int enc, 
  void* pCtx,
  int(*xCompare)(void*,int,const void*,int,const void*)
){
  int rc;
  db.mutex.Lock()
  assert( !db.mallocFailed );
  rc = createCollation(db, Name, (byte)enc, pCtx, xCompare, 0);
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

/*
** Register a new collation sequence with the database handle db.
*/
func int sqlite3_create_collation_v2(
  sqlite3* db, 
  const char *Name, 
  int enc, 
  void* pCtx,
  int(*xCompare)(void*,int,const void*,int,const void*),
  void(*xDel)(void*)
){
  int rc;
  db.mutex.Lock()
  assert( !db.mallocFailed );
  rc = createCollation(db, Name, (byte)enc, pCtx, xCompare, xDel);
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

/*
** Register a collation sequence factory callback with the database handle
** db. Replace any previously installed collation sequence factory.
*/
func int sqlite3_collation_needed(
  sqlite3 *db, 
  void *pCollNeededArg, 
  void(*xCollNeeded)(void*,sqlite3*,int eTextRep,const char*)
){
  db.mutex.Lock()
  db.xCollNeeded = xCollNeeded;
  db.xCollNeeded16 = 0;
  db.pCollNeededArg = pCollNeededArg;
  db.mutex.Unlock()
  return SQLITE_OK;
}

/*
** Test to see whether or not the database connection is in autocommit
** mode.  Return TRUE if it is and FALSE if not.  Autocommit mode is on
** by default.  Autocommit is disabled by a BEGIN statement and reenabled
** by the next COMMIT or ROLLBACK.
**
******* THIS IS AN EXPERIMENTAL API AND IS SUBJECT TO CHANGE ******
*/
func int sqlite3_get_autocommit(sqlite3 *db){
  return db.autoCommit;
}

/*
** The following routines are subtitutes for constants SQLITE_CORRUPT,
** SQLITE_MISUSE, SQLITE_CANTOPEN, SQLITE_IOERR and possibly other error
** constants.  They server two purposes:
**
**   1.  Serve as a convenient place to set a breakpoint in a debugger
**       to detect when version error conditions occurs.
**
**   2.  Invoke sqlite3_log() to provide the source code location where
**       a low-level error is first detected.
*/
 int sqlite3CorruptError(int lineno){
  sqlite3_log(SQLITE_CORRUPT,
              "database corruption at line %d of [%.10s]",
              lineno, 20+sqlite3_sourceid());
  return SQLITE_CORRUPT;
}
 int sqlite3MisuseError(int lineno){
  sqlite3_log(SQLITE_MISUSE, 
              "misuse at line %d of [%.10s]",
              lineno, 20+sqlite3_sourceid());
  return SQLITE_MISUSE;
}
 int sqlite3CantopenError(int lineno){
  sqlite3_log(SQLITE_CANTOPEN, 
              "cannot open file at line %d of [%.10s]",
              lineno, 20+sqlite3_sourceid());
  return SQLITE_CANTOPEN;
}


/*
** Return meta information about a specific column of a database table.
** See comment in sqlite3.h (sqlite.h.in) for details.
*/
#ifdef SQLITE_ENABLE_COLUMN_METADATA
func int sqlite3_table_column_metadata(
  sqlite3 *db,                /* Connection handle */
  const char *zDbName,        /* Database name or NULL */
  const char *zTableName,     /* Table name */
  const char *zColumnName,    /* Column name */
  char const **pzDataType,    /* OUTPUT: Declared data type */
  char const **pzCollSeq,     /* OUTPUT: Collation sequence name */
  int *pNotNull,              /* OUTPUT: True if NOT NULL constraint exists */
  int *pPrimaryKey,           /* OUTPUT: True if column part of PK */
  int *pAutoinc               /* OUTPUT: True if column is auto-increment */
){
  int rc;
  char *zErrMsg = 0;
  Table *pTab = 0;
  Column *pCol = 0;
  int iCol;

  char const *zDataType = 0;
  char const *zCollSeq = 0;
  int notnull = 0;
  int primarykey = 0;
  int autoinc = 0;

  /* Ensure the database schema has been loaded */
  db.mutex.Lock()
  db.LockAll()
  rc = db.Init(zErrMsg)
  if( SQLITE_OK!=rc ){
    goto error_out;
  }

  /* Locate the table in question */
  pTab = db.FindTable(zTableName, zDbName)
  if( !pTab || pTab.Select ){
    pTab = 0;
    goto error_out;
  }

  /* Find the column for which info is requested */
  if( sqlite3IsRowid(zColumnName) ){
    iCol = pTab.iPKey;
    if( iCol>=0 ){
      pCol = &pTab.Columns[iCol];
    }
  }else{
    for(iCol=0; iCol<pTab.nCol; iCol++){
      pCol = &pTab.Columns[iCol];
      if CaseInsensitiveMatch(pCol.Name, zColumnName) {
        break;
      }
    }
    if( iCol==pTab.nCol ){
      pTab = 0;
      goto error_out;
    }
  }

  /* The following block stores the meta information that will be returned
  ** to the caller in local variables zDataType, zCollSeq, notnull, primarykey
  ** and autoinc. At this point there are two possibilities:
  ** 
  **     1. The specified column name was rowid", "oid" or "_rowid_" 
  **        and there is no explicitly declared IPK column. 
  **
  **     2. The table is not a view and the column name identified an 
  **        explicitly declared column. Copy meta information from *pCol.
  */ 
  if( pCol ){
    zDataType = pCol.zType;
    zCollSeq = pCol.zColl;
    notnull = pCol.notNull!=0;
    primarykey  = pCol.isPrimKey!=0;
    autoinc = pTab.iPKey==iCol && (pTab.tabFlags & TF_Autoincrement)!=0;
  }else{
    zDataType = "INTEGER";
    primarykey = 1;
  }
  if( !zCollSeq ){
    zCollSeq = "BINARY";
  }

error_out:
  db.LeaveBtreeAll()

  /* Whether the function call succeeded or failed, set the output parameters
  ** to whatever their local counterparts contain. If an error did occur,
  ** this has the effect of zeroing all output parameters.
  */
  if( pzDataType ) *pzDataType = zDataType;
  if( pzCollSeq ) *pzCollSeq = zCollSeq;
  if( pNotNull ) *pNotNull = notnull;
  if( pPrimaryKey ) *pPrimaryKey = primarykey;
  if( pAutoinc ) *pAutoinc = autoinc;

  if( SQLITE_OK==rc && !pTab ){
    zErrMsg = fmt.Sprintf("no such table column: %v.%v", zTableName, zColumnName);
    rc = SQLITE_ERROR;
  }
  db.Error(rc, (zErrMsg?"%s":0), zErrMsg);
  zErrMsg = nil
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}
#endif

/*
** Sleep for a little while.  Return the amount of time slept.
*/
func int sqlite3_sleep(int ms){
  sqlite3_vfs *pVfs;
  int rc;
  pVfs = sqlite3_vfs_find(0);
  if( pVfs==0 ) return 0;

  /* This function works in milliseconds, but the underlying OsSleep() 
  ** API uses microseconds. Hence the 1000's.
  */
  rc = (sqlite3OsSleep(pVfs, 1000*ms)/1000);
  return rc;
}

/*
** Enable or disable the extended result codes.
*/
func int sqlite3_extended_result_codes(sqlite3 *db, int onoff){
  db.mutex.Lock()
  db.errMask = onoff ? 0xffffffff : 0xff;
  db.mutex.Unlock()
  return SQLITE_OK;
}

/*
** Invoke the xFileControl method on a particular database.
*/
func int sqlite3_file_control(sqlite3 *db, const char *zDbName, int op, void *pArg){
  int rc = SQLITE_ERROR;
  Btree *pBtree;

  db.mutex.Lock()
  pBtree = db.DbNameToBtree(zDbName)
  if( pBtree ){
    Pager *pPager;
    sqlite3_file *fd;
    pBtree.Lock()
    pPager = pBtree.Pager()
    assert( pPager!=0 );
    fd = sqlite3PagerFile(pPager);
    assert( fd!=0 );
    if( op==SQLITE_FCNTL_FILE_POINTER ){
      *(sqlite3_file**)pArg = fd;
      rc = SQLITE_OK;
    }else if( fd.pMethods ){
      rc = sqlite3OsFileControl(fd, op, pArg);
    }else{
      rc = SQLITE_NOTFOUND;
    }
    pBtree.Unlock()
  }
  db.mutex.Unlock()
  return rc;   
}

/*
** This is a utility routine, useful to VFS implementations, that checks
** to see if a database file was a URI that contained a specific query 
** parameter, and if so obtains the value of the query parameter.
**
** The zFilename argument is the filename pointer passed into the xOpen()
** method of a VFS implementation.  The zParam argument is the name of the
** query parameter we seek.  This routine returns the value of the zParam
** parameter if it exists.  If the parameter does not exist, this routine
** returns a NULL pointer.
*/
func const char *sqlite3_uri_parameter(const char *zFilename, const char *zParam){
  if( zFilename==0 ) return 0;
  zFilename += sqlite3Strlen30(zFilename) + 1;
  while( zFilename[0] ){
    int x = strcmp(zFilename, zParam);
    zFilename += sqlite3Strlen30(zFilename) + 1;
    if( x==0 ) return zFilename;
    zFilename += sqlite3Strlen30(zFilename) + 1;
  }
  return 0;
}

/*
** Return a boolean value for a query parameter.
*/
func int sqlite3_uri_boolean(const char *zFilename, const char *zParam, int bDflt){
  const char *z = sqlite3_uri_parameter(zFilename, zParam);
  bDflt = bDflt!=0;
  return z ? sqlite3GetBoolean(z, bDflt) : bDflt;
}

/*
** Return a 64-bit integer value for a query parameter.
*/
func int64 sqlite3_uri_int64(
  const char *zFilename,    /* Filename as passed to xOpen */
  const char *zParam,       /* URI parameter sought */
  int64 bDflt       /* return if parameter is missing */
){
	if v, e := strconv.ParseInt(sqlite3_uri_parameter(zFilename, zParam), 0, 64); e == nil {
		bDflt = v
	}
	return bDflt
}

//	Return the Btree pointer identified by zDbName. Return nil if not found.
func (db *sqlite3) BbNameToBtree(DbName string) *Btree {
	for i, _ := range db.Databases {
		if db.Databases[i].pBt && (DbName == "" || CaseInsensitiveMatch(DbName, db.Databases[i].Name)) {
			return db.Databases[i].pBt
		}
	}
	return nil
}

/*
** Return the filename of the database associated with a database
** connection.
*/
func const char *sqlite3_db_filename(sqlite3 *db, const char *zDbName){
  pBt := db.DbNameToBtree(zDbName)
  return pBt ? sqlite3BtreeGetFilename(pBt) : 0;
}

/*
** Return 1 if database is read-only or 0 if read/write.  Return -1 if
** no such database exists.
*/
func int sqlite3_db_readonly(sqlite3 *db, const char *zDbName){
  pBt := db.DbNameToBtree(zDbName)
  return pBt ? sqlite3PagerIsreadonly(pBt.Pager()) : -1;
}
