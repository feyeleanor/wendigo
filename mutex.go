//	This file contains the C functions that implement mutexes.
//
//	This file contains code that is common across all mutex implementations.

//	For debugging purposes, record when the mutex subsystem is initialized and uninitialized so that we can assert() if there is an attempt to
//	allocate a mutex while the system is uninitialized.
static int mutexIsInit = 0;


//	Initialize the mutex system.
 int sqlite3MutexInit(void){ 
  int rc = SQLITE_OK;
  if( !sqlite3GlobalConfig.mutex.xMutexAlloc ){
    /* If the xMutexAlloc method has not been set, then the user did not
    ** install a mutex implementation via sqlite3_config() prior to 
    ** Initialize() being called. This block copies pointers to
    ** the default implementation into the sqlite3GlobalConfig structure.
    */
    sqlite3_mutex_methods const *pFrom;
    sqlite3_mutex_methods *pTo = &sqlite3GlobalConfig.mutex;

    if( sqlite3GlobalConfig.bCoreMutex ){
      pFrom = sqlite3DefaultMutex();
    }else{
      pFrom = sqlite3NoopMutex();
    }
    memcpy(pTo, pFrom, offsetof(sqlite3_mutex_methods, xMutexAlloc));
    memcpy(&pTo.xMutexFree, &pFrom.xMutexFree,
           sizeof(*pTo) - offsetof(sqlite3_mutex_methods, xMutexFree));
    pTo.xMutexAlloc = pFrom.xMutexAlloc;
  }
  rc = sqlite3GlobalConfig.mutex.xMutexInit();
  return rc;
}

/*
** Shutdown the mutex system. This call frees resources allocated by
** sqlite3MutexInit().
*/
 int sqlite3MutexEnd(void){
  int rc = SQLITE_OK;
  if( sqlite3GlobalConfig.mutex.xMutexEnd ){
    rc = sqlite3GlobalConfig.mutex.xMutexEnd();
  }
  return rc;
}

/*
** Retrieve a pointer to a static mutex or allocate a new dynamic one.
*/
func sqlite3_mutex *sqlite3_mutex_alloc(int id){
  return sqlite3GlobalConfig.mutex.xMutexAlloc(id);
}

 sqlite3_mutex *sqlite3MutexAlloc(int id){
  if( !sqlite3GlobalConfig.bCoreMutex ){
    return 0;
  }
  assert( mutexIsInit );
  return sqlite3GlobalConfig.mutex.xMutexAlloc(id);
}

//	Free a dynamic mutex.
func (p *sqlite3_mutex) Free() {
	if p != nil {
		sqlite3GlobalConfig.mutex.xMutexFree(p)
	}
}

//	Obtain the mutex p. If some other thread already has the mutex, block until it can be obtained.
func (p *sqlite3_mutex) Lock() {
	if p != nil {
		sqlite3GlobalConfig.mutex.xLock(p)
	}
}

//	Obtain the mutex p. If successful, return SQLITE_OK. Otherwise, if another thread holds the mutex and it cannot be obtained, return SQLITE_BUSY.
func (p *sqlite3_mutex) TestLock() (rc int) {
	rc = SQLITE_OK
	if p != nil {
		return sqlite3GlobalConfig.mutex.xMutexTry(p)
	}
	return
}

//	The Unlock() routine exits a mutex that was previously entered by the same thread. The behavior is undefined if the mutex 
//	is not currently entered. If a NULL pointer is passed as an argument this function is a no-op.
func (p *sqlite3_mutex) Unlock() {
	if p != nil {
		sqlite3GlobalConfig.mutex.xUnlock(p)
	}
}

func (p *sqlite3_mutex) CriticalSection(f func()) {
	defer p.Unlock()
	p.Lock()
	f()
}