/* This file contains OS interface code that is common to all
** architectures.
*/
#define _SQLITE_OS_C_ 1
#undef _SQLITE_OS_C_

/*
** The default SQLite sqlite3_vfs implementations do not allocate
** memory (actually, os_unix.c allocates a small amount of memory
** from within OsOpen()), but some third-party implementations may.
** So we test the effects of a malloc() failing and the sqlite3OsXXX()
** function returning SQLITE_IOERR_NOMEM using the DO_OS_MALLOC_TEST macro.
**
** The following functions are instrumented for malloc() failure 
** testing:
**
**     sqlite3OsRead()
**     sqlite3OsWrite()
**     sqlite3OsSync()
**     sqlite3OsFileSize()
**     sqlite3OsLock()
**     sqlite3OsCheckReservedLock()
**     sqlite3OsFileControl()
**     sqlite3OsShmMap()
**     sqlite3OsOpen()
**     sqlite3OsDelete()
**     sqlite3OsAccess()
**     sqlite3OsFullPathname()
**
*/
  #define DO_OS_MALLOC_TEST(x)

/*
** The following routines are convenience wrappers around methods
** of the sqlite3_file object.  This is mostly just syntactic sugar. All
** of this would be completely automatic if SQLite were coded using
** C++ instead of plain old C.
*/
 int sqlite3OsClose(sqlite3_file *pId){
  int rc = SQLITE_OK;
  if( pId.pMethods ){
    rc = pId.pMethods.xClose(pId);
    pId.pMethods = 0;
  }
  return rc;
}
 int sqlite3OsRead(sqlite3_file *id, void *pBuf, int amt, int64 offset){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xRead(id, pBuf, amt, offset);
}
 int sqlite3OsWrite(sqlite3_file *id, const void *pBuf, int amt, int64 offset){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xWrite(id, pBuf, amt, offset);
}
 int sqlite3OsTruncate(sqlite3_file *id, int64 size){
  return id.pMethods.xTruncate(id, size);
}
 int sqlite3OsSync(sqlite3_file *id, int flags){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xSync(id, flags);
}
 int sqlite3OsFileSize(sqlite3_file *id, int64 *pSize){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xFileSize(id, pSize);
}
 int sqlite3OsLock(sqlite3_file *id, int lockType){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xLock(id, lockType);
}
 int sqlite3OsUnlock(sqlite3_file *id, int lockType){
  return id.pMethods.xUnlock(id, lockType);
}
 int sqlite3OsCheckReservedLock(sqlite3_file *id, int *pResOut){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xCheckReservedLock(id, pResOut);
}

/*
** Use sqlite3OsFileControl() when we are doing something that might fail
** and we need to know about the failures.  Use sqlite3OsFileControlHint()
** when simply tossing information over the wall to the VFS and we do not
** really care if the VFS receives and understands the information since it
** is only a hint and can be safely ignored.  The sqlite3OsFileControlHint()
** routine has no return value since the return value would be meaningless.
*/
 int sqlite3OsFileControl(sqlite3_file *id, int op, void *pArg){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xFileControl(id, op, pArg);
}
 void sqlite3OsFileControlHint(sqlite3_file *id, int op, void *pArg){
  (void)id.pMethods.xFileControl(id, op, pArg);
}

 int sqlite3OsSectorSize(sqlite3_file *id){
  int (*xSectorSize)(sqlite3_file*) = id.pMethods.xSectorSize;
  return (xSectorSize ? xSectorSize(id) : SQLITE_DEFAULT_SECTOR_SIZE);
}
 int sqlite3OsDeviceCharacteristics(sqlite3_file *id){
  return id.pMethods.xDeviceCharacteristics(id);
}
 int sqlite3OsShmLock(sqlite3_file *id, int offset, int n, int flags){
  return id.pMethods.xShmLock(id, offset, n, flags);
}
 void sqlite3OsShmBarrier(sqlite3_file *id){
  id.pMethods.xShmBarrier(id);
}
 int sqlite3OsShmUnmap(sqlite3_file *id, int deleteFlag){
  return id.pMethods.xShmUnmap(id, deleteFlag);
}
 int sqlite3OsShmMap(
  sqlite3_file *id,               /* Database file handle */
  int iPage,
  int pgsz,
  int bExtend,                    /* True to extend file if necessary */
  void volatile **pp              /* OUT: Pointer to mapping */
){
  DO_OS_MALLOC_TEST(id);
  return id.pMethods.xShmMap(id, iPage, pgsz, bExtend, pp);
}

/*
** The next group of routines are convenience wrappers around the
** VFS methods.
*/
 int sqlite3OsOpen(
  sqlite3_vfs *pVfs, 
  const char *zPath, 
  sqlite3_file *pFile, 
  int flags, 
  int *pFlagsOut
){
  int rc;
  DO_OS_MALLOC_TEST(0);
  /* 0x87f7f is a mask of SQLITE_OPEN_ flags that are valid to be passed
  ** down into the VFS layer.  Some SQLITE_OPEN_ flags (for example,
  ** SQLITE_OPEN_FULLMUTEX or SQLITE_OPEN_SHAREDCACHE) are blocked before
  ** reaching the VFS. */
  rc = pVfs.xOpen(pVfs, zPath, pFile, flags & 0x87f7f, pFlagsOut);
  assert( rc==SQLITE_OK || pFile.pMethods==0 );
  return rc;
}
 int sqlite3OsDelete(sqlite3_vfs *pVfs, const char *zPath, int dirSync){
  DO_OS_MALLOC_TEST(0);
  assert( dirSync==0 || dirSync==1 );
  return pVfs.xDelete(pVfs, zPath, dirSync);
}
 int sqlite3OsAccess(
  sqlite3_vfs *pVfs, 
  const char *zPath, 
  int flags, 
  int *pResOut
){
  DO_OS_MALLOC_TEST(0);
  return pVfs.xAccess(pVfs, zPath, flags, pResOut);
}
 int sqlite3OsFullPathname(
  sqlite3_vfs *pVfs, 
  const char *zPath, 
  int nPathOut, 
  char *zPathOut
){
  DO_OS_MALLOC_TEST(0);
  zPathOut[0] = 0;
  return pVfs.xFullPathname(pVfs, zPath, nPathOut, zPathOut);
}
#ifndef SQLITE_OMIT_LOAD_EXTENSION
 void *sqlite3OsDlOpen(sqlite3_vfs *pVfs, const char *zPath){
  return pVfs.xDlOpen(pVfs, zPath);
}
 void sqlite3OsDlError(sqlite3_vfs *pVfs, int nByte, char *zBufOut){
  pVfs.xDlError(pVfs, nByte, zBufOut);
}
 void (*sqlite3OsDlSym(sqlite3_vfs *pVfs, void *pHdle, const char *zSym))(void){
  return pVfs.xDlSym(pVfs, pHdle, zSym);
}
 void sqlite3OsDlClose(sqlite3_vfs *pVfs, void *pHandle){
  pVfs.xDlClose(pVfs, pHandle);
}
#endif /* SQLITE_OMIT_LOAD_EXTENSION */
 int sqlite3OsRandomness(sqlite3_vfs *pVfs, int nByte, char *zBufOut){
  return pVfs.xRandomness(pVfs, nByte, zBufOut);
}
 int sqlite3OsSleep(sqlite3_vfs *pVfs, int nMicro){
  return pVfs.xSleep(pVfs, nMicro);
}
 int sqlite3OsCurrentTimeInt64(sqlite3_vfs *pVfs, int64 *pTimeOut){
  int rc;
  /* IMPLEMENTATION-OF: R-49045-42493 SQLite will use the xCurrentTimeInt64()
  ** method to get the current date and time if that method is available
  ** (if iVersion is 2 or greater and the function pointer is not NULL) and
  ** will fall back to xCurrentTime() if xCurrentTimeInt64() is
  ** unavailable.
  */
  if( pVfs.iVersion>=2 && pVfs.xCurrentTimeInt64 ){
    rc = pVfs.xCurrentTimeInt64(pVfs, pTimeOut);
  }else{
    double r;
    rc = pVfs.xCurrentTime(pVfs, &r);
    *pTimeOut = (int64)(r*86400000.0);
  }
  return rc;
}

 int sqlite3OsOpenMalloc(
  sqlite3_vfs *pVfs, 
  const char *zFile, 
  sqlite3_file **ppFile, 
  int flags,
  int *pOutFlags
){
  int rc = SQLITE_NOMEM;
  sqlite3_file *pFile;
  pFile = (sqlite3_file *)sqlite3MallocZero(pVfs.szOsFile);
  if( pFile ){
    rc = sqlite3OsOpen(pVfs, zFile, pFile, flags, pOutFlags);
    if( rc!=SQLITE_OK ){
      pFile = nil
    }else{
      *ppFile = pFile;
    }
  }
  return rc;
}
 int sqlite3OsCloseFree(sqlite3_file *pFile){
  int rc = SQLITE_OK;
  assert( pFile );
  rc = sqlite3OsClose(pFile);
  pFile = nil
  return rc;
}

/*
** This function is a wrapper around the OS specific implementation of
** sqlite3_os_init(). The purpose of the wrapper is to provide the
** ability to simulate a malloc failure, so that the handling of an
** error in sqlite3_os_init() by the upper layers can be tested.
*/
 int sqlite3OsInit(void){
  void *p = sqlite3_malloc(10);
  if( p==0 ) return SQLITE_NOMEM;
  p = nil
  return sqlite3_os_init();
}

/*
** The list of all registered VFS implementations.
*/
static sqlite3_vfs * vfsList = 0;

/*
** Locate a VFS by name.  If no name is given, simply return the
** first VFS on the list.
*/
func sqlite3_vfs *sqlite3_vfs_find(const char *zVfs){
  sqlite3_vfs *pVfs = 0;
  sqlite3_mutex *mutex;
  mutex = sqlite3MutexAlloc(SQLITE_MUTEX_STATIC_MASTER);
  mutex.Lock()
  for(pVfs = vfsList; pVfs; pVfs=pVfs.Next){
    if( zVfs==0 ) break;
    if( strcmp(zVfs, pVfs.Name)==0 ) break;
  }
  mutex.Unlock()
  return pVfs;
}

/*
** Unlink a VFS from the linked list
*/
static void vfsUnlink(sqlite3_vfs *pVfs){
  if( pVfs==0 ){
    /* No-op */
  }else if( vfsList==pVfs ){
    vfsList = pVfs.Next;
  }else if( vfsList ){
    sqlite3_vfs *p = vfsList;
    while( p.Next && p.Next!=pVfs ){
      p = p.Next;
    }
    if( p.Next==pVfs ){
      p.Next = pVfs.Next;
    }
  }
}

/*
** Register a VFS with the system.  It is harmless to register the same
** VFS multiple times.  The new VFS becomes the default if makeDflt is
** true.
*/
func int sqlite3_vfs_register(sqlite3_vfs *pVfs, int makeDflt){
  sqlite3_mutex *mutex
  mutex = sqlite3MutexAlloc(SQLITE_MUTEX_STATIC_MASTER)
  mutex.Lock()
  vfsUnlink(pVfs);
  if( makeDflt || vfsList==0 ){
    pVfs.Next = vfsList;
    vfsList = pVfs;
  }else{
    pVfs.Next = vfsList.Next;
    vfsList.Next = pVfs;
  }
  assert(vfsList);
  mutex.Unlock()
  return SQLITE_OK;
}

/*
** Unregister a VFS so that it is no longer accessible.
*/
func int sqlite3_vfs_unregister(sqlite3_vfs *pVfs){
  sqlite3_mutex *mutex = sqlite3MutexAlloc(SQLITE_MUTEX_STATIC_MASTER);
  mutex.Lock()
  vfsUnlink(pVfs);
  mutex.Unlock()
  return SQLITE_OK;
}
