//	This file implements the default page cache implementation (the sqlite3_pcache interface). It also contains part of the implementation of the SQLITE_CONFIG_PAGECACHE and ReleaseMemory() features. If the default page cache implementation is overriden, then neither of these two features are available.

//	Each page cache is an instance of the following object. Every open database file (including each in-memory database and each temporary or transient database) has a single page cache which is an instance of this object.
//	Pointers to structures of this type are cast and returned as  opaque sqlite3_pcache* handles.
type PCache1 struct {
	//	Cache configuration parameters. Page size (PageSize) and the purgeable flag (Purgeable) are set when the cache is created. nMax may be modified at any time by a call to the pcache1Cachesize() method.
	//	The Group mutex must be held when accessing nMax.
	*Group						//	Group this cache belongs to
  int PageSize;                         /* Size of allocated pages in bytes */
  int ExtraBytes;                        /* Size of extra space in bytes */
  int Purgeable;                     /* True if cache is purgeable */
  uint nMin;                  /* Minimum number of pages reserved */
  uint nMax;                  /* Configured "cache_size" value */
  uint n90pct;                /* nMax*9/10 */
  uint iMaxKey;               /* Largest key seen since xTruncate() */

  /* Hash table of all pages. The following variables may only be accessed
  ** when the accessor is holding the Group mutex.
  */
  uint nRecyclable;           /* Number of pages in the LRU list */
  uint nPage;                 /* Total number of pages in apHash */
  uint nHash;                 /* Number of slots in apHash[] */
  PgHdr1 **apHash;                    /* Hash table for fast lookup by key */
};

//	Each cache entry is represented by an instance of the following structure. Unless SQLITE_PCACHE_SEPARATE_HEADER is defined, a buffer of PgHdr1.pCache.PageSize bytes is allocated directly before this structure in memory.
struct PgHdr1 {
  sqlite3_pcache_page page;
  uint iKey;             /* Key value (page number) */
  PgHdr1 *Next;                 /* Next in hash table chain */
  PCache1 *pCache;               /* Cache that currently owns this page */
  PgHdr1 *pLruNext;              /* Next in LRU list of unpinned pages */
  PgHdr1 *pLruPrev;              /* Previous in LRU list of unpinned pages */
};

//	Free slots in the allocator used to divide up the buffer provided using the SQLITE_CONFIG_PAGECACHE mechanism.
struct PgFreeslot {
	PgFreeslot *Next;  /* Next free slot */
};

//	Global data used by this cache.
static struct PCacheGlobal {
  Group grp;                    /* The global Group for mode (2) */

	//	Variables related to SQLITE_CONFIG_PAGECACHE settings. The szSlot, nSlot, pStart, pEnd, nReserve, and isInit values are all fixed at Initialize() time and do not require mutex protection. The nFreeSlot and pFree values do require mutex protection.
	isInit	bool				//	True if initialized
  int szSlot;                    /* Size of each free slot */
  int nSlot;                     /* The number of pcache slots */
  int nReserve;                  /* Try to keep nFreeSlot above this */
  void *pStart, *pEnd;           /* Bounds of pagecache malloc range */
  /* Above requires no mutex.  Use mutex below for variable that follow. */
  sqlite3_mutex *mutex;          /* Mutex for accessing the following: */
  PgFreeslot *pFree;             /* Free page blocks */
  int nFreeSlot;                 /* Number of unused pcache slots */
  /* The following value requires a mutex to change.  We skip the mutex on
  ** reading because (1) most platforms read a 32-bit integer atomically and
  ** (2) even if an incorrect value is read, no great harm is done since this
  ** is really just an optimization. */
  int bUnderPressure;            /* True if low on PAGECACHE memory */
} pcache1_g;

/*
** All code in this file should access the global structure above via the
** alias "pcache1". This ensures that the WSD emulation is used when
** compiling for systems that do not support real WSD.
*/
#define pcache1 pcache1_g

/******************************************************************************/
/******** Page Allocation/SQLITE_CONFIG_PCACHE Related Functions **************/

//	This function is called during initialization if a static buffer is supplied to use for the page-cache by passing the SQLITE_CONFIG_PAGECACHE verb to sqlite3_config(). Parameter pBuf points to an allocation large enough to contain 'n' buffers of 'sz' bytes each.
//	This routine is called from Initialize() and so it is guaranteed to be serialized already. There is no need for further mutexing.
void sqlite3PCacheBufferSetup(void *pBuf, int sz, int n){
	if pcache1.isInit {
		PgFreeslot *p;
		sz = ROUNDDOWN(sz, 8)
		pcache1.szSlot = sz
		pcache1.nSlot = pcache1.nFreeSlot = n
		pcache1.nReserve = n > 90 ? 10 : (n / 10 + 1)
		pcache1.pStart = pBuf
		pcache1.pFree = 0
		pcache1.bUnderPressure = 0
		while( n-- ){
			p = (PgFreeslot*)pBuf
			p.Next = pcache1.pFree
			pcache1.pFree = p
			pBuf = (void*)&((char*)pBuf)[sz]
		}
		pcache1.pEnd = pBuf
	}
}

/*
** Malloc function used within this file to allocate space from the buffer
** configured using sqlite3_config(SQLITE_CONFIG_PAGECACHE) option. If no 
** such buffer exists or there is no space left in it, this function falls 
** back to sqlite3Malloc().
**
** Multiple threads can run this routine at the same time.  Global variables
** in pcache1 need to be protected via mutex.
*/
static void *pcache1Alloc(int nByte){
  void *p = 0;
  sqlite3StatusSet(SQLITE_STATUS_PAGECACHE_SIZE, nByte);
  if( nByte<=pcache1.szSlot ){
    pcache1.mutex.Lock()
    p = (PgHdr1 *)pcache1.pFree;
    if( p ){
      pcache1.pFree = pcache1.pFree.Next;
      pcache1.nFreeSlot--;
      pcache1.bUnderPressure = pcache1.nFreeSlot<pcache1.nReserve;
      assert( pcache1.nFreeSlot>=0 );
      sqlite3StatusAdd(SQLITE_STATUS_PAGECACHE_USED, 1);
    }
    pcache1.mutex.Unlock()
  }
  if( p==0 ){
    /* Memory is not available in the SQLITE_CONFIG_PAGECACHE pool.  Get
    ** it from sqlite3Malloc instead.
    */
    p = sqlite3Malloc(nByte);
    if( p ){
      int sz = sqlite3MallocSize(p);
      pcache1.mutex.Lock()
      sqlite3StatusAdd(SQLITE_STATUS_PAGECACHE_OVERFLOW, sz);
      pcache1.mutex.Unlock()
    }
  }
  return p;
}

/*
** Free an allocated buffer obtained from pcache1Alloc().
*/
static int pcache1Free(void *p){
  int nFreed = 0;
  if( p==0 ) return 0;
  if( p>=pcache1.pStart && p<pcache1.pEnd ){
    PgFreeslot *pSlot;
    pcache1.mutex.Lock()
    sqlite3StatusAdd(SQLITE_STATUS_PAGECACHE_USED, -1);
    pSlot = (PgFreeslot*)p;
    pSlot.Next = pcache1.pFree;
    pcache1.pFree = pSlot;
    pcache1.nFreeSlot++;
    pcache1.bUnderPressure = pcache1.nFreeSlot<pcache1.nReserve;
    assert( pcache1.nFreeSlot<=pcache1.nSlot );
    pcache1.mutex.Unlock()
  }else{
    nFreed = sqlite3MallocSize(p);
    pcache1.mutex.Lock()
    sqlite3StatusAdd(SQLITE_STATUS_PAGECACHE_OVERFLOW, -nFreed);
    pcache1.mutex.Unlock()
    p = nil
  }
  return nFreed;
}

#ifdef SQLITE_ENABLE_MEMORY_MANAGEMENT
//	Return the size of a pcache allocation
func pcache1MemSize(p *void) (size int) {
	if p >= pcache1.pStart && p < pcache1.pEnd {
		return pcache1.szSlot
	} else {
		size = sqlite3MallocSize(p)
		return size;
	}
}
#endif /* SQLITE_ENABLE_MEMORY_MANAGEMENT */

//	Allocate a new page object initially associated with cache pCache.
static PgHdr1 *pcache1AllocPage(cache *PCache1) {
	//	The group mutex must be released before pcache1Alloc() is called. This is because it may call ReleaseMemory(), which assumes that this mutex is not held.
	cache.Group.mutex.Unlock()
#ifdef SQLITE_PCACHE_SEPARATE_HEADER
	pPg := pcache1Alloc(cache.PageSize)
	p := sqlite3Malloc(sizeof(PgHdr1) + cache.ExtraBytes)
	if pPg == nil || p == nil {
		pcache1Free(pPg)
		p = nil
		pPg = 0
	}
#else
	pPg := pcache1Alloc(sizeof(PgHdr1) + cache.PageSize + cache.ExtraBytes)
	p := (PgHdr1 *)(&((byte *)pPg)[cache.PageSize])
#endif
	cache.Group.mutex.Lock()
	if pPg != nil {
		p.page.pBuf = pPg
		p.page.pExtra = &p[1]
		if cache.Purgeable {
			cache.Group.CurrentPage++
		}
		return p
	}
	return 0
}

//	Free a page object allocated by pcache1AllocPage().
//	The pointer is allowed to be NULL, which is prudent.
func pcache1FreePage(p *PgHdr1){
	if p != nil {
		cache := p.pCache
		pcache1Free(p.page.pBuf)
#ifdef SQLITE_PCACHE_SEPARATE_HEADER
		p = nil
#endif
		if cache.Purgeable {
			cache.Group.CurrentPage--;
		}
	}
}

//	Malloc function used by SQLite to obtain space from the buffer configured using sqlite3_config(SQLITE_CONFIG_PAGECACHE) option. If no such buffer exists, this function falls back to sqlite3Malloc().
void *sqlite3PageMalloc(sz int){
	return pcache1Alloc(sz)
}

//	Free an allocated buffer obtained from sqlite3PageMalloc().
void sqlite3PageFree(void *p) {
	pcache1Free(p)
}

//	Return true if it desirable to avoid allocating a new page cache entry.
//	If memory was allocated specifically to the page cache using SQLITE_CONFIG_PAGECACHE but that memory has all been used, then it is desirable to avoid allocating a new page cache entry because presumably SQLITE_CONFIG_PAGECACHE was suppose to be sufficient for all page cache needs and we should not need to spill the allocation onto the heap.
//	Or, the heap is used for all page cache memory but the heap is under memory pressure, then again it is desirable to avoid allocating a new page cache entry in order to avoid stressing the heap even further.
static int pcache1UnderMemoryPressure(cache *PCache1) {
	if pcache1.nSlot && (cache.PageSize + cache.ExtraBytes) <= pcache1.szSlot {
		return pcache1.bUnderPressure
	} else {
		return sqlite3HeapNearlyFull()
	}
}

//	This function is used to resize the hash table used by the cache passed as the first argument.
//	The PCache mutex must be held when this function is called.
func pcache1ResizeHash(p *PCache1) (rc int) {
	nNew := p.nHash * 2
	if nNew < 256 {
		nNew = 256
	}
	p.Group.mutex.Unlock()
	if p.nHash {}
	apNew := (PgHdr1 **)(sqlite3_malloc(sizeof(PgHdr1 *) * nNew))
	if p.nHash {}
	p.Group.mutex.Lock()
	if apNew {
		memset(apNew, 0, sizeof(PgHdr1 *) * nNew)
		for i := 0; i < p.nHash; i++ {
			page := p.apHash[i]
			for next := page; page != nil; {
				h := page.iKey % nNew
				next = page.Next
				page.Next = apNew[h]
				apNew[h] = page
			}
		}
		p.apHash = apNew
		p.nHash = nNew
	}
	if p.apHash == nil {
		rc = SQLITE_NOMEM
	} else {
		rc = SQLITE_OK
	}
	return
}

//	This function is used internally to remove the page pPage from the Group LRU list, if is part of it. If pPage is not part of the Group LRU list, then this function is a no-op.
//	The Group mutex must be held when this function is called.
//	If pPage is NULL then this routine is a no-op.
static void pcache1PinPage(page *PgHdr1) {
	if page != nil {
		cache := page.pCache
		group := cache.Group
		if page.pLruNext || page == group.LruTail {
			if page.pLruPrev {
				page.pLruPrev.pLruNext = page.pLruNext
			}
			if page.pLruNext {
				page.pLruNext.pLruPrev = page.pLruPrev
			}
			if group.LruHead == page {
				group.LruHead = page.pLruNext
			}
			if group.LruTail == page {
				group.LruTail = page.pLruPrev
			}
			page.pLruNext = 0
			page.pLruPrev = 0
			page.pCache.nRecyclable--
		}
	}
}


//	Remove the page supplied as an argument from the hash table (PCache1.apHash structure) that it is currently stored in.
//	The Group mutex must be held when this function is called.
static void pcache1RemoveFromHash(page *PgHdr1) {
	cache := page.pCache
	h := page.iKey % cache.nHash
	for pp := &pCache.apHash[h]; (*pp) != page; pp = &(*pp).Next {}
	*pp = (*pp).Next
	pCache.nPage--
}

/*
** Discard all pages from cache pCache with a page number (key value) 
** greater than or equal to iLimit. Any pinned pages that meet this 
** criteria are unpinned before they are discarded.
**
** The PCache mutex must be held when this function is called.
*/
static void pcache1TruncateUnsafe(
  PCache1 *pCache,             /* The cache to truncate */
  uint iLimit          /* Drop pages with this pgno or larger */
){
  uint h;
  for(h=0; h<pCache.nHash; h++){
    PgHdr1 **pp = &pCache.apHash[h]; 
    PgHdr1 *pPage;
    while( (pPage = *pp)!=0 ){
      if( pPage.iKey>=iLimit ){
        pCache.nPage--;
        *pp = pPage.Next;
        pcache1PinPage(pPage);
        pcache1FreePage(pPage);
      }else{
        pp = &pPage.Next;
      }
    }
  }
}

//	Implementation of the sqlite3_pcache.xInit method.
static int pcache1Init(void *NotUsed){
	assert( !pcache1.isInit )
	memset(&pcache1, 0, sizeof(pcache1))
	if sqlite3GlobalConfig.bCoreMutex {
		pcache1.grp.mutex = sqlite3_mutex_alloc(SQLITE_MUTEX_STATIC_LRU);
		pcache1.mutex = sqlite3_mutex_alloc(SQLITE_MUTEX_STATIC_PMEM);
	}
	pcache1.grp.Pinned = 10
	pcache1.isInit = true
	return SQLITE_OK
}

//	Implementation of the sqlite3_pcache.xShutdown method.
//	Note that the static mutex allocated in xInit does not need to be freed.
static void pcache1Shutdown(void *NotUsed){
	assert( pcache1.isInit )
	memset(&pcache1, 0, sizeof(pcache1))
}

//	Implementation of the sqlite3_pcache.xCreate method.
//	Allocate a new cache.
static sqlite3_pcache *pcache1Create(int PageSize, int ExtraBytes, int Purgeable) {
	group		*Group

	//	The seperateCache variable is true if each PCache has its own private Group. In other words, separateCache is true for mode (1) where no mutexing is required.
	//
	//		*  Always use a unified cache (mode-2) if ENABLE_MEMORY_MANAGEMENT
	//		*  Always use a unified cache in single-threaded applications
	//		*  Otherwise (if multi-threaded and ENABLE_MEMORY_MANAGEMENT is off) use separate caches (mode-1)

#if defined(SQLITE_ENABLE_MEMORY_MANAGEMENT)
	const int separateCache = 0
#else
	int separateCache = sqlite3GlobalConfig.bCoreMutex > 0
#endif

	assert( (PageSize & (PageSize - 1)) == 0 && PageSize >= 512 && PageSize <= 65536 )
	assert( ExtraBytes < 300 )

	sz := sizeof(PCache1) + sizeof(Group) * separateCache
	cache := (PCache1 *)(sqlite3_malloc(sz))
	if cache != nil {
		memset(cache, 0, sz)
		if separateCache {
			group = (Group*)(&cache[1])
			group.Pinned = 10
		} else {
			group = &pcache1.grp
		}
		cache.Group = group
		cache.PageSize = PageSize
		cache.ExtraBytes = ExtraBytes
		cache.Purgeable = Purgeable
		if Purgeable {
			cache.nMin = 10
			group.mutex.Lock()
			group.MinPage += cache.nMin
			group.Pinned = group.MaxPage + 10 - group.MinPage
			group.mutex.Unlock()
		}
	}
	return (sqlite3_pcache *)(cache)
}

//	Implementation of the sqlite3_pcache.xCachesize method.
//	Configure the cache_size limit for a cache.
static void pcache1Cachesize(sqlite3_pcache *p, int nMax) {
	cache := (PCache1 *)(p)
	if cache.Purgeable {
		group := cache.Group
		group.mutex.Lock()
		group.MaxPage += (nMax - pCache.nMax)
		group.Pinned = group.MaxPage + 10 - group.MinPage
		cache.nMax = nMax
		cache.n90pct = cache.nMax * 9 / 10
		group.EnforceMaxPage()
		group.mutex.Unlock()
	}
}

//	Implementation of the sqlite3_pcache.xShrink method. 
//	Free up as much memory as possible.
static void pcache1Shrink(sqlite3_pcache *p) {
	cache := (PCache1*)(p)
	if cache.Purgeable {
		group := cache.Group
		group.mutex.Lock()
		savedMaxPage := group.MaxPage
		group.MaxPage = 0
		group.EnforceMaxPage()
		group.MaxPage = savedMaxPage
		group.mutex.Unlock()
	}
}

//	Implementation of the sqlite3_pcache.xPagecount method.
static int pcache1Pagecount(sqlite3_pcache *p) {
	cache := (PCache1*)(p)
	cache.Group.mutex.Lock()
	n := cache.nPage
	cache.Group.mutex.Unlock()
	return n
}

//	Implementation of the sqlite3_pcache.xFetch method. 
//	Fetch a page by key value.
//
//	Whether or not a new page may be allocated by this function depends on the value of the createFlag argument. 0 means do not allocate a new page. 1 means allocate a new page if space is easily available. 2 means to try really hard to allocate a new page.
//	For a non-purgeable cache (a cache used as the storage for an in-memory database) there is really no difference between createFlag 1 and 2. So the calling function (pcache.c) will never have a createFlag of 1 on a non-purgeable cache.
//
//	There are three different approaches to obtaining space for a page, depending on the value of parameter createFlag (which may be 0, 1 or 2).
//
//		1. Regardless of the value of createFlag, the cache is searched for a copy of the requested page. If one is found, it is returned.
//
//		2. If createFlag==0 and the page is not already in the cache, NULL is returned.
//
//		3. If createFlag is 1, and the page is not already in the cache, then return NULL (do not allocate a new page) if any of the following conditions are true:
//
//			(a) the number of pages pinned by the cache is greater than PCache1.nMax, or
//
//			(b) the number of pages pinned by the cache is greater than the sum of nMax for all purgeable caches, less the sum of nMin for all other purgeable caches, or
//
//		4. If none of the first three conditions apply and the cache is marked as purgeable, and if one of the following is true:
//
//			(a) The number of pages allocated for the cache is already PCache1.nMax, or
//
//			(b) The number of pages allocated for all purgeable caches is already equal to or greater than the sum of nMax for all purgeable caches,
//
//			(c) The system is under memory pressure and wants to avoid unnecessary pages cache entry allocations
//
//		then attempt to recycle a page from the LRU list. If it is the right size, return the recycled buffer. Otherwise, free the buffer and proceed to step 5. 
//
//		5. Otherwise, allocate and return a new page buffer.
static sqlite3_pcache_page *pcache1Fetch(
  sqlite3_pcache *p, 
  uint iKey, 
  createFlag bool
){
	page 		*PgHdr1

	cache := (PCache1 *)(p)
	assert( cache.Purgeable || createFlag!=1 )
	assert( cache.Purgeable || cache.nMin==0 )
	assert( cache.Purgeable==0 || cache.nMin==10 )
	assert( cache.nMin==0 || cache.Purgeable )

	group := cache.Group
	group.mutex.Lock()

	//	Step 1: Search the hash table for an existing entry.
	if cache.nHash > 0 {
		h := iKey % cache.nHash
		for page = cache.apHash[h]; page && page.iKey != iKey; page = page.Next {}
	}

	//	Step 2: Abort if no existing page is found and createFlag is 0
	if page || !createFlag {
		pcache1PinPage(page)
		goto fetch_out
	}

	//	Step 3: Abort if createFlag is 1 but the cache is nearly full
	assert( cache.nPage >= cache.nRecyclable )
	pinned := cache.nPage - cache.nRecyclable
	assert( group.Pinned == group.MaxPage + 10 - group.MinPage )
	assert( cache.n90pct == cache.nMax * 9 / 10 )
	if createFlag && (pinned >= group.Pinned || pinned >= cache.n90pct || pcache1UnderMemoryPressure(cache)) {
		goto fetch_out
	}

	if cache.nPage >= cache.nHash && pcache1ResizeHash(cache) {
		goto fetch_out
	}

	 //	Step 4. Try to recycle a page. */
	if( cache.Purgeable && group.LruTail && ((cache.nPage + 1 >= cache.nMax) || group.CurrentPage >= group.MaxPage || pcache1UnderMemoryPressure(cache))) {
		page = group.LruTail
		pcache1RemoveFromHash(page)
		pcache1PinPage(page)
		candidate := page.pCache

		//	We want to verify that PageSize and ExtraBytes are the same for pOther and pCache. Assert that we can verify this by comparing sums.
		assert( (cache.PageSize & (cache.PageSize - 1)) == 0 && cache.PageSize >= 512 )
		assert( cache.ExtraBytes < 512 )
		assert( (candidate.PageSize & (candidate.PageSize - 1)) == 0 && candidate.PageSize >= 512 )
		assert( candidate.ExtraBytes < 512 )

		if candidate.PageSize + candidate.ExtraBytes != cache.PageSize + cache.ExtraBytes {
			pcache1FreePage(page)
			page = nil
		} else {
			group.CurrentPage -= (candidate.Purgeable - cache.Purgeable)
		}
	}

	//	Step 5. If a usable page buffer has still not been found, attempt to allocate a new one. 
	if page == nil {
		if createFlag {}
		page = pcache1AllocPage(cache)
		if createFlag {}
	}

	if page {
		h := iKey % cache.nHash
		cache.nPage++
		page.iKey = iKey
		page.Next = cache.apHash[h]
		page.pCache = cache
		page.pLruPrev = nil
		page.pLruNext = nil
		*(void **)page.page.pExtra = 0
		cache.apHash[h] = page
	}

fetch_out:
	if page && iKey > cache.iMaxKey {
		cache.iMaxKey = iKey
	}
	group.mutex.Unlock()
	return &page.page
}


//	Implementation of the sqlite3_pcache.xUnpin method.
//
//	Mark a page as unpinned (eligible for asynchronous recycling).
static void pcache1Unpin(
  sqlite3_pcache *p, 
  sqlite3_pcache_page *pPg, 
  int reuseUnlikely
){
	cache := (PCache1 *)(p)
	page := (PgHdr1 *)(pPg)
	group := cache.Group;
 
	assert( page.pCache == cache )
	group.mutex.Lock()

	//	It is an error to call this function if the page is already part of the Group LRU list.
	assert( page.pLruPrev == nil && page.pLruNext == nil )
	assert( group.LruHead != page && group.LruTail != page )

	if reuseUnlikely || group.CurrentPage > group.MaxPage {
		pcache1RemoveFromHash(page)
		pcache1FreePage(page)
	} else {
		//	Add the page to the Group LRU list.
		if group.LruHead {
			group.LruHead.pLruPrev = page
			page.pLruNext = group.LruHead
			group.LruHead = page
		} else {
			group.LruTail = page
			group.LruHead = page
		}
		cache.nRecyclable++
	}
	cache.Group.mutex.Unlock()
}

//	Implementation of the sqlite3_pcache.xRekey method.
static void pcache1Rekey(
  sqlite3_pcache *p,
  sqlite3_pcache_page *pPg,
  uint iOld,
  uint iNew
){
	PgHdr1 **pp;
	cache := (PCache1 *)(p)
	page := (PgHdr1 *)(pPg)
	assert( page.iKey == iOld )
	assert( page.pCache == cache )

	cache.Group.mutex.Lock()

	h := iOld % cache.nHash
	pp = &cache.apHash[h]
	for (*pp) != page {
		pp = &(*pp).Next
	}
	*pp = page.Next

	h = iNew % cache.nHash
	page.iKey = iNew
	page.Next = cache.apHash[h]
	cache.apHash[h] = page
	if iNew > cache.iMaxKey {
		cache.iMaxKey = iNew
	}
	cache.Group.mutex.Unlock()
}

//	Implementation of the sqlite3_pcache.xTruncate method.
//	Discard all unpinned pages in the cache with a page number equal to or greater than parameter iLimit. Any pinned pages with a page number equal to or greater than iLimit are implicitly unpinned.
static void pcache1Truncate(sqlite3_pcache *p, uint iLimit){
	cache := (PCache1 *)(p)
	cache.Group.mutex.Lock()
	if iLimit <= cache.iMaxKey {
		pcache1TruncateUnsafe(cache, iLimit)
		cache.iMaxKey = iLimit - 1
	}
	cache.Group.mutex.Unlock()
}

//	Implementation of the sqlite3_pcache.xDestroy method. 
//	Destroy a cache allocated using pcache1Create().
static void pcache1Destroy(sqlite3_pcache *p){
	cache := (PCache1 *)(p)
	group := cache.Group
	assert( cache.Purgeable || (cache.nMax == 0 && cache.nMin == 0) )
	group.mutex.Lock()
	pcache1TruncateUnsafe(cache, 0)
	assert( group.MaxPage >= cache.nMax )
	group.MaxPage -= cache.nMax
	assert( group.MinPage >= cache.nMin )
	group.MinPage -= cache.nMin
	group.Pinned = group.MaxPage + 10 - group.MinPage
	group.EnforceMaxPage()
	group.mutex.Unlock()
	cache.apHash = nil
	cache = nil
}

//	This function is called during initialization (Initialize()) to install the default pluggable cache module, assuming the user has not already provided an alternative.
void sqlite3PCacheSetDefault(void){
  static const sqlite3_pcache_methods2 defaultMethods = {
    1,                       /* iVersion */
    0,                       /* pArg */
    pcache1Init,             /* xInit */
    pcache1Shutdown,         /* xShutdown */
    pcache1Create,           /* xCreate */
    pcache1Cachesize,        /* xCachesize */
    pcache1Pagecount,        /* xPagecount */
    pcache1Fetch,            /* xFetch */
    pcache1Unpin,            /* xUnpin */
    pcache1Rekey,            /* xRekey */
    pcache1Truncate,         /* xTruncate */
    pcache1Destroy,          /* xDestroy */
    pcache1Shrink            /* xShrink */
  };
  sqlite3_config(SQLITE_CONFIG_PCACHE2, &defaultMethods);
}

#ifdef SQLITE_ENABLE_MEMORY_MANAGEMENT
//	This function is called to free superfluous dynamically allocated memory held by the pager system.
//	nReq is the number of bytes of memory required. Once this much has been released, the function returns. The return value is the total number of bytes of memory released.

func sqlite3PcacheReleaseMemory(nReq int) int {
	int	nFree = 0
	if pcache1.pStart == nil {
		&pcache1.grp.mutex.CriticalSection(func() {
			for p := pcache1.grp.LruTail; (nReq < 0 || nFree < nReq) && (pcache1.grp.LruTail != nil); p = pcache1.grp.LruTail {
				nFree += pcache1MemSize(p.page.pBuf)
#ifdef SQLITE_PCACHE_SEPARATE_HEADER
				nFree += sqlite3MemSize(p)
#endif
				pcache1PinPage(p)
				pcache1RemoveFromHash(p)
				pcache1FreePage(p)
			}
		})
	}
	return nFree
}
#endif /* SQLITE_ENABLE_MEMORY_MANAGEMENT */