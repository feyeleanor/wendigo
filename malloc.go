//	Attempt to release up to n bytes of non-essential memory currently held by SQLite. An example of non-essential memory is memory used to cache database pages that are not currently in use.
func ReleaseMemory(int n) int {
#ifdef SQLITE_ENABLE_MEMORY_MANAGEMENT
	return sqlite3PcacheReleaseMemory(n)
#else
	//	IMPLEMENTATION-OF: R-34391-24921 The ReleaseMemory() routine is a no-op returning zero if SQLite is not compiled with SQLITE_ENABLE_MEMORY_MANAGEMENT.
	return 0
#endif
}

/*
** An instance of the following object records the location of
** each unused scratch buffer.
*/
typedef struct ScratchFreeslot {
  struct ScratchFreeslot *Next;   /* Next unused scratch buffer */
} ScratchFreeslot;

/*
** State information local to the memory allocation subsystem.
*/
static struct Mem0Global {
  sqlite3_mutex *mutex;         /* Mutex to serialize access */

  /*
  ** The alarm callback and its arguments.  The mem0.mutex lock will
  ** be held while the callback is running.  Recursive calls into
  ** the memory subsystem are allowed, but no new callbacks will be
  ** issued.
  */
  int64 alarmThreshold;
  void (*alarmCallback)(void*, int64,int);
  void *alarmArg;

  /*
  ** Pointers to the end of sqlite3GlobalConfig.pScratch memory
  ** (so that a range test can be used to determine if an allocation
  ** being freed came from pScratch) and a pointer to the list of
  ** unused scratch allocations.
  */
  void *pScratchEnd;
  ScratchFreeslot *pScratchFree;
  uint32 nScratchFree;

  //	True if heap is nearly "full" where "full" is defined by the sqlite3_soft_heap_limit64() setting.
  int nearlyFull;
} mem0 = { 0, 0, 0, 0, 0, 0, 0, 0 };

/*
** This routine runs when the memory allocator sees that the
** total memory allocation is about to exceed the soft heap
** limit.
*/
static void softHeapLimitEnforcer(
  void *NotUsed, 
  int64 NotUsed2,
  int allocSize
){
  ReleaseMemory(allocSize);
}

/*
** Change the alarm callback
*/
static int sqlite3MemoryAlarm(
  void(*xCallback)(void *pArg, int64 used,int N),
  void *pArg,
  int64 iThreshold
){
  int nUsed;
  mem0.mutex.Lock()
  mem0.alarmCallback = xCallback;
  mem0.alarmArg = pArg;
  mem0.alarmThreshold = iThreshold;
  nUsed = sqlite3StatusValue(SQLITE_STATUS_MEMORY_USED);
  mem0.nearlyFull = (iThreshold>0 && iThreshold<=nUsed);
  mem0.mutex.Unlock()
  return SQLITE_OK;
}

/*
** Set the soft heap-size limit for the library. Passing a zero or 
** negative value indicates no limit.
*/
func int64 sqlite3_soft_heap_limit64(int64 n){
  int64 priorLimit;
  int64 excess;
  mem0.mutex.Lock()
  priorLimit = mem0.alarmThreshold;
  mem0.mutex.Unlock()
  if( n<0 ) return priorLimit;
  if( n>0 ){
    sqlite3MemoryAlarm(softHeapLimitEnforcer, 0, n);
  }else{
    sqlite3MemoryAlarm(0, 0, 0);
  }
  excess = sqlite3_memory_used() - n;
  if( excess>0 ) ReleaseMemory((int)(excess & 0x7fffffff));
  return priorLimit;
}

/*
** Initialize the memory allocation subsystem.
*/
 int sqlite3MallocInit(void){
  if( sqlite3GlobalConfig.m.xMalloc==0 ){
    sqlite3MemSetDefault();
  }
  memset(&mem0, 0, sizeof(mem0));
  if( sqlite3GlobalConfig.bCoreMutex ){
    mem0.mutex = sqlite3MutexAlloc(SQLITE_MUTEX_STATIC_MEM);
  }
  if( sqlite3GlobalConfig.pScratch && sqlite3GlobalConfig.szScratch>=100
      && sqlite3GlobalConfig.nScratch>0 ){
    int i, n, sz;
    ScratchFreeslot *pSlot;
    sz = ROUNDDOWN(sqlite3GlobalConfig.szScratch, 8)
    sqlite3GlobalConfig.szScratch = sz;
    pSlot = (ScratchFreeslot*)sqlite3GlobalConfig.pScratch;
    n = sqlite3GlobalConfig.nScratch;
    mem0.pScratchFree = pSlot;
    mem0.nScratchFree = n;
    for(i=0; i<n-1; i++){
      pSlot.Next = (ScratchFreeslot*)(sz+(char*)pSlot);
      pSlot = pSlot.Next;
    }
    pSlot.Next = 0;
    mem0.pScratchEnd = (void*)&pSlot[1];
  }else{
    mem0.pScratchEnd = 0;
    sqlite3GlobalConfig.pScratch = 0;
    sqlite3GlobalConfig.szScratch = 0;
    sqlite3GlobalConfig.nScratch = 0;
  }
  if( sqlite3GlobalConfig.pPage==0 || sqlite3GlobalConfig.PageSize<512
      || sqlite3GlobalConfig.nPage<1 ){
    sqlite3GlobalConfig.pPage = 0;
    sqlite3GlobalConfig.PageSize = 0;
    sqlite3GlobalConfig.nPage = 0;
  }
  return sqlite3GlobalConfig.m.xInit(sqlite3GlobalConfig.m.pAppData);
}

//	Return true if the heap is currently under memory pressure - in other words if the amount of heap used is close to the limit set by	sqlite3_soft_heap_limit64().
 int sqlite3HeapNearlyFull(void){
  return mem0.nearlyFull;
}

/*
** Deinitialize the memory allocation subsystem.
*/
 void sqlite3MallocEnd(void){
  if( sqlite3GlobalConfig.m.xShutdown ){
    sqlite3GlobalConfig.m.xShutdown(sqlite3GlobalConfig.m.pAppData);
  }
  memset(&mem0, 0, sizeof(mem0));
}

/*
** Return the amount of memory currently checked out.
*/
func int64 sqlite3_memory_used(void){
  int n, mx;
  int64 res;
  sqlite3_status(SQLITE_STATUS_MEMORY_USED, &n, &mx, 0);
  res = (int64)n;  /* Work around bug in Borland C. Ticket #3216 */
  return res;
}

/*
** Return the maximum amount of memory that has ever been
** checked out since either the beginning of this process
** or since the most recent reset.
*/
func int64 sqlite3_memory_highwater(int resetFlag){
  int n, mx;
  int64 res;
  sqlite3_status(SQLITE_STATUS_MEMORY_USED, &n, &mx, resetFlag);
  res = (int64)mx;  /* Work around bug in Borland C. Ticket #3216 */
  return res;
}

/*
** Trigger the alarm 
*/
static void sqlite3MallocAlarm(int nByte){
  void (*xCallback)(void*,int64,int);
  int64 nowUsed;
  void *pArg;
  if( mem0.alarmCallback==0 ) return;
  xCallback = mem0.alarmCallback;
  nowUsed = sqlite3StatusValue(SQLITE_STATUS_MEMORY_USED);
  pArg = mem0.alarmArg;
  mem0.alarmCallback = 0;
  mem0.mutex.Unlock()
  xCallback(pArg, nowUsed, nByte);
  mem0.mutex.Lock()
  mem0.alarmCallback = xCallback;
  mem0.alarmArg = pArg;
}

/*
** Do a memory allocation with statistics and alarms.  Assume the
** lock is already held.
*/
static int mallocWithAlarm(int n, void **pp){
  int nFull;
  void *p;
  nFull = sqlite3GlobalConfig.m.xRoundup(n);
  sqlite3StatusSet(SQLITE_STATUS_MALLOC_SIZE, n);
  if( mem0.alarmCallback!=0 ){
    int nUsed = sqlite3StatusValue(SQLITE_STATUS_MEMORY_USED);
    if( nUsed >= mem0.alarmThreshold - nFull ){
      mem0.nearlyFull = 1;
      sqlite3MallocAlarm(nFull);
    }else{
      mem0.nearlyFull = 0;
    }
  }
  p = sqlite3GlobalConfig.m.xMalloc(nFull);
#ifdef SQLITE_ENABLE_MEMORY_MANAGEMENT
  if( p==0 && mem0.alarmCallback ){
    sqlite3MallocAlarm(nFull);
    p = sqlite3GlobalConfig.m.xMalloc(nFull);
  }
#endif
  if( p ){
    nFull = sqlite3MallocSize(p);
    sqlite3StatusAdd(SQLITE_STATUS_MEMORY_USED, nFull);
    sqlite3StatusAdd(SQLITE_STATUS_MALLOC_COUNT, 1);
  }
  *pp = p;
  return nFull;
}

/*
** Allocate memory.  This routine is like sqlite3_malloc() except that it
** assumes the memory subsystem has already been initialized.
*/
 void *sqlite3Malloc(int n){
  void *p;
  if( n<=0               /* IMP: R-65312-04917 */ 
   || n>=0x7fffff00
  ){
    /* A memory allocation of a number of bytes which is near the maximum
    ** signed integer value might cause an integer overflow inside of the
    ** xMalloc().  Hence we limit the maximum size to 0x7fffff00, giving
    ** 255 bytes of overhead.  SQLite itself will never use anything near
    ** this amount.  The only way to reach the limit is with sqlite3_malloc() */
    p = 0;
  }else if( sqlite3GlobalConfig.bMemstat ){
    mem0.mutex.Lock()
    mallocWithAlarm(n, &p);
    mem0.mutex.Unlock()
  }else{
    p = sqlite3GlobalConfig.m.xMalloc(n);
  }
  assert( EIGHT_BYTE_ALIGNMENT(p) );  /* IMP: R-04675-44850 */
  return p;
}

/*
** This version of the memory allocation is for use by the application.
** First make sure the memory subsystem is initialized, then do the
** allocation.
*/
func void *sqlite3_malloc(int n){
  return sqlite3Malloc(n);
}

/*
** Allocate memory that is to be used and released right away.
** This routine is similar to alloca() in that it is not intended
** for situations where the memory might be held long-term.  This
** routine is intended to get memory to old large transient data
** structures that would not normally fit on the stack of an
** embedded processor.
*/
 void *sqlite3ScratchMalloc(int n){
  void *p;
  assert( n>0 );

  mem0.mutex.Lock()
  if( mem0.nScratchFree && sqlite3GlobalConfig.szScratch>=n ){
    p = mem0.pScratchFree;
    mem0.pScratchFree = mem0.pScratchFree.Next;
    mem0.nScratchFree--;
    sqlite3StatusAdd(SQLITE_STATUS_SCRATCH_USED, 1);
    sqlite3StatusSet(SQLITE_STATUS_SCRATCH_SIZE, n);
    mem0.mutex.Unlock()
  }else{
    if( sqlite3GlobalConfig.bMemstat ){
      sqlite3StatusSet(SQLITE_STATUS_SCRATCH_SIZE, n);
      n = mallocWithAlarm(n, &p);
      if( p ) sqlite3StatusAdd(SQLITE_STATUS_SCRATCH_OVERFLOW, n);
      mem0.mutex.Unlock()
    }else{
      mem0.mutex.Unlock()
      p = sqlite3GlobalConfig.m.xMalloc(n);
    }
  }
  return p;
}
 void sqlite3ScratchFree(void *p){
  if( p ){
    if( p>=sqlite3GlobalConfig.pScratch && p<mem0.pScratchEnd ){
      /* Release memory from the SQLITE_CONFIG_SCRATCH allocation */
      ScratchFreeslot *pSlot;
      pSlot = (ScratchFreeslot*)p;
      mem0.mutex.Lock()
      pSlot.Next = mem0.pScratchFree;
      mem0.pScratchFree = pSlot;
      mem0.nScratchFree++;
      assert( mem0.nScratchFree <= (uint32)sqlite3GlobalConfig.nScratch );
      sqlite3StatusAdd(SQLITE_STATUS_SCRATCH_USED, -1);
      mem0.mutex.Unlock()
    }else{
      /* Release memory back to the heap */
      if( sqlite3GlobalConfig.bMemstat ){
        int iSize = sqlite3MallocSize(p);
        mem0.mutex.Lock()
        sqlite3StatusAdd(SQLITE_STATUS_SCRATCH_OVERFLOW, -iSize);
        sqlite3StatusAdd(SQLITE_STATUS_MEMORY_USED, -iSize);
        sqlite3StatusAdd(SQLITE_STATUS_MALLOC_COUNT, -1);
        sqlite3GlobalConfig.m.xFree(p);
        mem0.mutex.Unlock()
      }else{
        sqlite3GlobalConfig.m.xFree(p);
      }
    }
  }
}

/*
** Return the size of a memory allocation previously obtained from
** sqlite3Malloc() or sqlite3_malloc().
*/
 int sqlite3MallocSize(void *p){
  return sqlite3GlobalConfig.m.xSize(p);
}
 int sqlite3DbMallocSize(sqlite3 *db, void *p){
    return sqlite3GlobalConfig.m.xSize(p);
}

//	Change the size of an existing memory allocation
void *sqlite3Realloc(void *pOld, int nBytes){
  int nOld, nNew, nDiff;
  void *pNew;
  if( pOld==0 ){
    return sqlite3Malloc(nBytes); /* IMP: R-28354-25769 */
  }
  if( nBytes<=0 ){
    pOld = nil /* IMP: R-31593-10574 */
    return 0;
  }
  if( nBytes>=0x7fffff00 ){
    /* The 0x7ffff00 limit term is explained in comments on sqlite3Malloc() */
    return 0;
  }
  nOld = sqlite3MallocSize(pOld);
  /* IMPLEMENTATION-OF: R-46199-30249 SQLite guarantees that the second
  ** argument to xRealloc is always a value returned by a prior call to
  ** xRoundup. */
  nNew = sqlite3GlobalConfig.m.xRoundup(nBytes);
  if( nOld==nNew ){
    pNew = pOld;
  }else if( sqlite3GlobalConfig.bMemstat ){
    mem0.mutex.Lock()
    sqlite3StatusSet(SQLITE_STATUS_MALLOC_SIZE, nBytes);
    nDiff = nNew - nOld;
    if( sqlite3StatusValue(SQLITE_STATUS_MEMORY_USED) >= 
          mem0.alarmThreshold-nDiff ){
      sqlite3MallocAlarm(nDiff);
    }
    pNew = sqlite3GlobalConfig.m.xRealloc(pOld, nNew);
    if( pNew==0 && mem0.alarmCallback ){
      sqlite3MallocAlarm(nBytes);
      pNew = sqlite3GlobalConfig.m.xRealloc(pOld, nNew);
    }
    if( pNew ){
      nNew = sqlite3MallocSize(pNew);
      sqlite3StatusAdd(SQLITE_STATUS_MEMORY_USED, nNew-nOld);
    }
    mem0.mutex.Unlock()
  }else{
    pNew = sqlite3GlobalConfig.m.xRealloc(pOld, nNew);
  }
  assert( EIGHT_BYTE_ALIGNMENT(pNew) ); /* IMP: R-04675-44850 */
  return pNew;
}

/*
** The public interface to sqlite3Realloc.  Make sure that the memory
** subsystem is initialized prior to invoking sqliteRealloc.
*/
func void *sqlite3_realloc(void *pOld, int n){
  return sqlite3Realloc(pOld, n);
}


/*
** Allocate and zero memory.
*/ 
 void *sqlite3MallocZero(int n){
  void *p = sqlite3Malloc(n);
  if( p ){
    memset(p, 0, n);
  }
  return p;
}

/*
** Allocate and zero memory.  If the allocation fails, make
** the mallocFailed flag in the connection pointer.
*/
 void *sqlite3DbMallocZero(sqlite3 *db, int n){
  void *p = sqlite3DbMallocRaw(db, n);
  if( p ){
    memset(p, 0, n);
  }
  return p;
}

/*
** Allocate and zero memory.  If the allocation fails, make
** the mallocFailed flag in the connection pointer.
**
** If db!=0 and db.mallocFailed is true (indicating a prior malloc
** failure on the same database connection) then always return 0.
** Hence for a particular database connection, once malloc starts
** failing, it fails consistently until mallocFailed is reset.
** This is an important assumption.  There are many places in the
** code that do things like this:
**
**         int *a = (int*)sqlite3DbMallocRaw(db, 100);
**         int *b = (int*)sqlite3DbMallocRaw(db, 200);
**         if( b ) a[10] = 9;
**
** In other words, if a subsequent malloc (ex: "b") worked, it is assumed
** that all prior mallocs (ex: "a") worked too.
*/
 void *sqlite3DbMallocRaw(sqlite3 *db, int n){
  void *p;
  assert( db==0 || db.pnBytesFreed==0 );
  if( db && db.mallocFailed ){
    return 0;
  }
  p = sqlite3Malloc(n);
  if( !p && db ){
    db.mallocFailed = true
  }
  return p;
}

/*
** Resize the block of memory pointed to by p to n bytes. If the
** resize fails, set the mallocFailed flag in the connection object.
*/
 void *sqlite3DbRealloc(sqlite3 *db, void *p, int n){
  void *pNew = 0;
  assert( db!=0 );
  if( !db.mallocFailed ){
    if( p==0 ){
      return sqlite3DbMallocRaw(db, n);
    }
      pNew = sqlite3_realloc(p, n);
      if( !pNew ){
        db.mallocFailed = true
      }
  }
  return pNew;
}

/*
** Attempt to reallocate p.  If the reallocation fails, then free p
** and set the mallocFailed flag in the database connection.
*/
void *sqlite3DbReallocOrFree(sqlite3 *db, void *p, int n){
	void *pNew;
	if pNew = sqlite3DbRealloc(db, p, n); pNew == nil {
		p = nil
	}
	return pNew;
}

/*
** Make a copy of a string in memory obtained from sqliteMalloc(). These 
** functions call sqlite3MallocRaw() directly instead of sqliteMalloc(). This
** is because when memory debugging is turned on, these two functions are 
** called via macros that record the current file and line number in the
** ThreadData structure.
*/
 char *sqlite3DbStrDup(sqlite3 *db, const char *z){
  char *zNew;
  size_t n;
  if( z==0 ){
    return 0;
  }
  n = sqlite3Strlen30(z) + 1;
  assert( (n&0x7fffffff)==n );
  zNew = sqlite3DbMallocRaw(db, (int)n);
  if( zNew ){
    memcpy(zNew, z, n);
  }
  return zNew;
}
 char *sqlite3DbStrNDup(sqlite3 *db, const char *z, int n){
  char *zNew;
  if( z==0 ){
    return 0;
  }
  assert( (n&0x7fffffff)==n );
  zNew = sqlite3DbMallocRaw(db, n+1);
  if( zNew ){
    memcpy(zNew, z, n);
    zNew[n] = 0;
  }
  return zNew;
}

//	This function must be called before exiting any API function (i.e. returning control to the user) that has called sqlite3_malloc or sqlite3_realloc.
//	The returned value is normally a copy of the second argument to this function. However, if a malloc() failure has occurred since the previous invocation SQLITE_NOMEM is returned instead. 
//	If the first argument, db, is not NULL and a malloc() error has occurred, then the connection error-code (the value returned by sqlite3_errcode()) is set to SQLITE_NOMEM.
func (db *sqlite3) ApiExit(rc int) int {
	//	If the db handle is not NULL, then we must hold the connection handle mutex here. Otherwise the read (and possible write) of db.mallocFailed is unsafe, as is the call to Error().
	if db != nil && (db.mallocFailed || rc == SQLITE_IOERR_NOMEM) {
		db.Error(SQLITE_NOMEM, "")
		db.mallocFailed = false
		rc = SQLITE_NOMEM
	}
	if rc == SQLITE_OK {
		rc &= 0xff
	} else {
		rc &= db.errMask
	}
	return
}