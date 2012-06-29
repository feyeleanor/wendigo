/* This file contains low-level memory allocation drivers for when
** SQLite will use the standard C-library malloc/realloc/free interface
** to obtain the memory it needs.
**
** This file contains implementations of the low-level memory allocation
** routines specified in the sqlite3_mem_methods object.  The content of
** this file is only used if SQLITE_SYSTEM_MALLOC is defined.  The
** default configuration is to use memory allocation routines in this
** file.
**
** C-preprocessor macro summary:
**
**    HAVE_MALLOC_USABLE_SIZE     The configure script sets this symbol if
**                                the malloc_usable_size() interface exists
**                                on the target platform.  Or, this symbol
**                                can be set manually, if desired.
**                                If an equivalent interface exists by
**                                a different name, using a separate -D
**                                option to rename it.
**
**    SQLITE_WITHOUT_ZONEMALLOC   Some older macs lack support for the zone
**                                memory allocator.  Set this symbol to enable
**                                building on older macs.
**
**    SQLITE_WITHOUT_MSIZE        Set this symbol to disable the use of
**                                _msize() on windows systems.  This might
**                                be necessary when compiling for Delphi,
**                                for example.
*/

/*
** This version of the memory allocator is the default.  It is
** used when no other memory allocator is specified using compile-time
** macros.
*/
#ifdef SQLITE_SYSTEM_MALLOC

#if (defined(HAVE_MALLOC_H) && defined(HAVE_MALLOC_USABLE_SIZE))
# include <malloc.h>    /* Needed for malloc_usable_size on linux */
#endif
#ifdef HAVE_MALLOC_USABLE_SIZE
# ifndef malloc_usable_size
#  define malloc_usable_size(x) malloc_usable_size(x)
# endif
#else
# undef malloc_usable_size
#endif

/*
** Like malloc(), but remember the size of the allocation
** so that we can find it later using sqlite3MemSize().
**
** For this low-level routine, we are guaranteed that nByte>0 because
** cases of nByte<=0 will be intercepted and dealt with by higher level
** routines.
*/
static void *sqlite3MemMalloc(int nByte){
#ifdef malloc_usable_size
  void *p = malloc( nByte );
  if( p==0 ){
    sqlite3_log(SQLITE_NOMEM, "failed to allocate %u bytes of memory", nByte);
  }
  return p;
#else
  int64 *p;
  assert( nByte>0 );
  nByte = ROUND(nByte, 8);
  p = malloc( nByte+8 );
  if( p ){
    p[0] = nByte;
    p++;
  }else{
    sqlite3_log(SQLITE_NOMEM, "failed to allocate %u bytes of memory", nByte);
  }
  return (void *)p;
#endif
}

/*
** Like free() but works for allocations obtained from sqlite3MemMalloc()
** or sqlite3MemRealloc().
**
** For this low-level routine, we already know that pPrior!=0 since
** cases where pPrior==0 will have been intecepted and dealt with
** by higher-level routines.
*/
static void sqlite3MemFree(void *pPrior){
  free(pPrior);
}

/*
** Report the allocated size of a prior return from xMalloc()
** or xRealloc().
*/
static int sqlite3MemSize(void *pPrior){
#ifdef malloc_usable_size
  return pPrior ? (int)malloc_usable_size(pPrior) : 0;
#else
  int64 *p;
  if( pPrior==0 ) return 0;
  p = (int64*)pPrior;
  p--;
  return (int)p[0];
#endif
}

/*
** Like realloc().  Resize an allocation previously obtained from
** sqlite3MemMalloc().
**
** For this low-level interface, we know that pPrior!=0.  Cases where
** pPrior==0 while have been intercepted by higher-level routine and
** redirected to xMalloc.  Similarly, we know that nByte>0 becauses
** cases where nByte<=0 will have been intercepted by higher-level
** routines and redirected to xFree.
*/
static void *sqlite3MemRealloc(void *pPrior, int nByte){
#ifdef malloc_usable_size
  void *p = SQLITE_REALLOC(pPrior, nByte);
  if( p==0 ){
    sqlite3_log(SQLITE_NOMEM,
      "failed memory resize %u to %u bytes",
      malloc_usable_size(pPrior), nByte);
  }
  return p;
#else
  int64 *p = (int64*)pPrior;
  assert( pPrior!=0 && nByte>0 );
  assert( nByte==ROUND(nByte, 8) ); /* EV: R-46199-30249 */
  p--;
  p = SQLITE_REALLOC(p, nByte+8 );
  if( p ){
    p[0] = nByte;
    p++;
  }else{
    sqlite3_log(SQLITE_NOMEM,
      "failed memory resize %u to %u bytes",
      sqlite3MemSize(pPrior), nByte);
  }
  return (void*)p;
#endif
}

/*
** Round up a request size to the next valid allocation size.
*/
static int sqlite3MemRoundup(int n){
  return ROUND(n, 8);
}

/*
** Initialize this module.
*/
static int sqlite3MemInit(void *NotUsed){
  return SQLITE_OK;
}

/*
** Deinitialize this module.
*/
static void sqlite3MemShutdown(void *NotUsed){
  return;
}

/*
** This routine is the only routine in this file with external linkage.
**
** Populate the low-level memory allocation function pointers in
** sqlite3GlobalConfig.m with pointers to the routines in this file.
*/
 void sqlite3MemSetDefault(void){
  static const sqlite3_mem_methods defaultMethods = {
     sqlite3MemMalloc,
     sqlite3MemFree,
     sqlite3MemRealloc,
     sqlite3MemSize,
     sqlite3MemRoundup,
     sqlite3MemInit,
     sqlite3MemShutdown,
     0
  };
  sqlite3_config(SQLITE_CONFIG_MALLOC, &defaultMethods);
}

#endif /* SQLITE_SYSTEM_MALLOC */
