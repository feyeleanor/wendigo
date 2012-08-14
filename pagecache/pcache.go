//	Some of the assert() macros in this code are too expensive to run even during normal debugging. Use them only rarely on long-running tests. Enable the expensive asserts using the -DSQLITE_ENABLE_EXPENSIVE_ASSERT=1 compile-time option.
#ifdef SQLITE_ENABLE_EXPENSIVE_ASSERT
# define expensive_assert(X)  assert(X)
#else
# define expensive_assert(X)
#endif

/********************************** Linked List Management ********************/

/*
** Remove page pPage from the list of dirty pages.
*/
static void pcacheRemoveFromDirtyList(PgHdr *pPage){
  PCache *p = pPage.pCache;

  assert( pPage.pDirtyNext || pPage==p.pDirtyTail );
  assert( pPage.pDirtyPrev || pPage==p.pDirty );

  /* Update the PCache1.pSynced variable if necessary. */
  if( p.pSynced==pPage ){
    PgHdr *pSynced = pPage.pDirtyPrev;
    while( pSynced && (pSynced.flags&PGHDR_NEED_SYNC) ){
      pSynced = pSynced.pDirtyPrev;
    }
    p.pSynced = pSynced;
  }

  if( pPage.pDirtyNext ){
    pPage.pDirtyNext.pDirtyPrev = pPage.pDirtyPrev;
  }else{
    assert( pPage==p.pDirtyTail );
    p.pDirtyTail = pPage.pDirtyPrev;
  }
  if( pPage.pDirtyPrev ){
    pPage.pDirtyPrev.pDirtyNext = pPage.pDirtyNext;
  }else{
    assert( pPage==p.pDirty );
    p.pDirty = pPage.pDirtyNext;
  }
  pPage.pDirtyNext = nil
  pPage.pDirtyPrev = nil

  expensive_assert( pcacheCheckSynced(p) );
}

/*
** Add page pPage to the head of the dirty list (PCache1.pDirty is set to
** pPage).
*/
static void pcacheAddToDirtyList(PgHdr *pPage){
  PCache *p = pPage.pCache;

  assert( pPage.pDirtyNext == nil && pPage.pDirtyPrev == nil && p.pDirty!=pPage );

  pPage.pDirtyNext = p.pDirty;
  if( pPage.pDirtyNext ){
    assert( pPage.pDirtyNext.pDirtyPrev == nil );
    pPage.pDirtyNext.pDirtyPrev = pPage;
  }
  p.pDirty = pPage;
  if( !p.pDirtyTail ){
    p.pDirtyTail = pPage;
  }
  if( !p.pSynced && 0==(pPage.flags&PGHDR_NEED_SYNC) ){
    p.pSynced = pPage;
  }
  expensive_assert( pcacheCheckSynced(p) );
}

/*
** Wrapper around the pluggable caches xUnpin method. If the cache is
** being used for an in-memory database, this function is a no-op.
*/
static void pcacheUnpin(PgHdr *p){
  PCache *pCache = p.pCache;
  if( pCache.Purgeable ){
    if( p.pgno==1 ){
      pCache.pPage1 = 0;
    }
    sqlite3GlobalConfig.pcache2.xUnpin(pCache.pCache, p.pPage, 0);
  }
}

/*************************************************** General Interfaces ******
**
** Initialize and shutdown the page cache subsystem. Neither of these
** functions are threadsafe.
*/
 int sqlite3PcacheInitialize(void){
  if( sqlite3GlobalConfig.pcache2.xInit==0 ){
    /* IMPLEMENTATION-OF: R-26801-64137 If the xInit() method is NULL, then the
    ** built-in default page cache is used instead of the application defined
    ** page cache. */
    sqlite3PCacheSetDefault();
  }
  return sqlite3GlobalConfig.pcache2.xInit(sqlite3GlobalConfig.pcache2.pArg);
}
 void sqlite3PcacheShutdown(void){
  if( sqlite3GlobalConfig.pcache2.xShutdown ){
    /* IMPLEMENTATION-OF: R-26000-56589 The xShutdown() method may be NULL. */
    sqlite3GlobalConfig.pcache2.xShutdown(sqlite3GlobalConfig.pcache2.pArg);
  }
}

//	Decrement the reference count on a page. If the page is clean and the reference count drops to 0, then it is made elible for recycling.
func (p *PgHdr) CacheRelease() {
void sqlite3PcacheRelease(PgHdr *p){
	assert( p.nRef > 0 )
	if p.nRef--; p.nRef == 0 {
		pCache = p.pCache
		pCache.nRef--
		if p.flags & PGHDR_DIRTY == 0 {
			pcacheUnpin(p)
		} else {
			//	Move the page to the head of the dirty list.
			pcacheRemoveFromDirtyList(p)
			pcacheAddToDirtyList(p)
		}
	}
}

/*
** Increase the reference count of a supplied page by 1.
*/
 void sqlite3PcacheRef(PgHdr *p){
  assert(p.nRef>0);
  p.PageSize;
}

/*
** Drop a page from the cache. There must be exactly one reference to the
** page. This function deletes that reference, so after it returns the
** page pointed to by p is invalid.
*/
 void sqlite3PcacheDrop(PgHdr *p){
  PCache *pCache;
  assert( p.nRef==1 );
  if( p.flags&PGHDR_DIRTY ){
    pcacheRemoveFromDirtyList(p);
  }
  pCache = p.pCache;
  pCache.nRef--;
  if( p.pgno==1 ){
    pCache.pPage1 = 0;
  }
  sqlite3GlobalConfig.pcache2.xUnpin(pCache.pCache, p.pPage, 1);
}

/*
** Make sure the page is marked as dirty. If it isn't dirty already,
** make it so.
*/
 void sqlite3PcacheMakeDirty(PgHdr *p){
  p.flags &= ~PGHDR_DONT_WRITE;
  assert( p.nRef>0 );
  if( 0==(p.flags & PGHDR_DIRTY) ){
    p.flags |= PGHDR_DIRTY;
    pcacheAddToDirtyList( p);
  }
}

/*
** Make sure the page is marked as clean. If it isn't clean already,
** make it so.
*/
 void sqlite3PcacheMakeClean(PgHdr *p){
  if( (p.flags & PGHDR_DIRTY) ){
    pcacheRemoveFromDirtyList(p);
    p.flags &= ~(PGHDR_DIRTY|PGHDR_NEED_SYNC);
    if( p.nRef==0 ){
      pcacheUnpin(p);
    }
  }
}

/*
** Change the page number of page p to newPageNumber.
*/
 void sqlite3PcacheMove(PgHdr *p, PageNumber newPageNumber){
  PCache *pCache = p.pCache;
  assert( p.nRef>0 );
  assert( newPageNumber>0 );
  sqlite3GlobalConfig.pcache2.xRekey(pCache.pCache, p.pPage, p.pgno,newPageNumber);
  p.pgno = newPageNumber;
  if( (p.flags&PGHDR_DIRTY) && (p.flags&PGHDR_NEED_SYNC) ){
    pcacheRemoveFromDirtyList(p);
    pcacheAddToDirtyList(p);
  }
}

/*
** Merge two lists of pages connected by pDirty and in pgno order.
** Do not both fixing the pDirtyPrev pointers.
*/
static PgHdr *pcacheMergeDirtyList(PgHdr *pA, PgHdr *pB){
  PgHdr result, *pTail;
  pTail = &result;
  while( pA && pB ){
    if( pA.pgno<pB.pgno ){
      pTail.pDirty = pA;
      pTail = pA;
      pA = pA.pDirty;
    }else{
      pTail.pDirty = pB;
      pTail = pB;
      pB = pB.pDirty;
    }
  }
  if( pA ){
    pTail.pDirty = pA;
  }else if( pB ){
    pTail.pDirty = pB;
  }else{
    pTail.pDirty = nil
  }
  return result.pDirty;
}

/*
** Sort the list of pages in accending order by pgno.  Pages are
** connected by pDirty pointers.  The pDirtyPrev pointers are
** corrupted by this sort.
**
** Since there cannot be more than 2^31 distinct pages in a database,
** there cannot be more than 31 buckets required by the merge sorter.
** One extra bucket is added to catch overflow in case something
** ever changes to make the previous sentence incorrect.
*/
#define N_SORT_BUCKET  32
static PgHdr *pcacheSortDirtyList(PgHdr *pIn){
  PgHdr *a[N_SORT_BUCKET], *p;
  int i;
  memset(a, 0, sizeof(a));
  while( pIn ){
    p = pIn;
    pIn = p.pDirty;
    p.pDirty = nil
    for(i=0; i<N_SORT_BUCKET-1; i++){
      if( a[i]==0 ){
        a[i] = p;
        break;
      }else{
        p = pcacheMergeDirtyList(a[i], p);
        a[i] = 0;
      }
    }
    if( i==N_SORT_BUCKET - 1 ){
      /* To get here, there need to be 2^(N_SORT_BUCKET) elements in
      ** the input list.  But that is impossible.
      */
      a[i] = pcacheMergeDirtyList(a[i], p);
    }
  }
  p = a[0];
  for(i=1; i<N_SORT_BUCKET; i++){
    p = pcacheMergeDirtyList(p, a[i]);
  }
  return p;
}


/*
** Return the number of references to the page supplied as an argument.
*/
 int sqlite3PcachePageRefcount(PgHdr *p){
  return p.nRef;
}
