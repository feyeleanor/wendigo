//	A complete page cache is an instance of this structure.
type PCache struct {
  PgHdr *pDirty, *pDirtyTail;         /* List of dirty pages in LRU order */
  PgHdr *pSynced;                     /* Last synced page in dirty page list */
  int nRef;                           /* Number of referenced pages */
  int size;                        /* Configured cache size */
  int PageSize;                         /* Size of every page in this cache */
  int ExtraBytes;                        /* Size of extra space for each page */
  int Purgeable;                     /* True if pages are on backing store */
  int (*xStress)(void*,PgHdr*);       /* Call to try make a page clean */
  void *pStress;                      /* Argument to xStress */
  sqlite3_pcache *pCache;             /* Pluggable cache module */
  PgHdr *pPage1;                      /* Reference to page 1 */
}

//	Return the size in bytes of a PCache object.
//	NONSENSE IN A GO PROGRAM
func sqlite3PcacheSize() int {
	return sizeof(PCache)
}

//	Change the page size for PCache object. The caller must ensure that there are no outstanding page references when this function is called.
func (p *PCache) SetPageSize(size int) {
	assert( p.nRef == 0 && !p.pDirty )
	if p.pCache {
		sqlite3GlobalConfig.pcache2.xDestroy(pCache.pCache);
		p.pCache = nil
		p.pPage1 = nil
	}
	p.size = size
}

//	Compute the number of pages of cache requested.
func (p *PCache) NumberOfCachePages() (count int) {
	if p.size >= 0 {
		count = p.size
	} else {
		count =  int((-1024 * int64(p.size)) / int64(p.PageSize + p.ExtraBytes))
	}
	return
}

//	Try to obtain a page from the cache.
func (pCache *PCache) Fetch(pgno PageNumber, createFlag bool) (page *PgHdr, rc int) {
	sqlite3_pcache_page *pPage = 0;
	PgHdr *pPgHdr = 0;

	assert( pCache != nil );
	assert( pgno > 0 );

	//	If the pluggable cache (sqlite3_pcache*) has not been allocated, allocate it now.
	if pCache.pCache == nil && createFlag {
		if p := sqlite3GlobalConfig.pcache2.xCreate(pCache.PageSize, pCache.ExtraBytes + sizeof(PgHdr), pCache.Purgeable); p == nil {
			return SQLITE_NOMEM
		}
		sqlite3GlobalConfig.pcache2.xCachesize(p, pCache.NumberOfCachePages())
		pCache.pCache = p
	}

	eCreate := createFlag * (1 + (!pCache.Purgeable || !pCache.pDirty));
	if pCache.pCache {
		pPage = sqlite3GlobalConfig.pcache2.xFetch(pCache.pCache, pgno, eCreate);
	}

	if pPage == nil && eCreate == 1 {
		PgHdr *pPg;

		//	Find a dirty page to write-out and recycle. First try to find a page that does not require a journal-sync (one with PGHDR_NEED_SYNC cleared), but if that is not possible settle for any other unreferenced dirty page.
		expensive_assert( pcacheCheckSynced(pCache) )
		for pPg = pCache.pSynced; pPg != nil && pPg.nRef || (pPg.flags & PGHDR_NEED_SYNC)); pPg = pPg.pDirtyPrev) {}
		pCache.pSynced = pPg
		if pPg == nil {
			for pPg = pCache.pDirtyTail; pPg != nil && pPg.nRef; pPg = pPg.pDirtyPrev) {}
		}
		if pPg != nil {
			if rc = pCache.xStress(pCache.pStress, pPg); rc != SQLITE_OK && rc != SQLITE_BUSY {
				return
			}
		}
		pPage = sqlite3GlobalConfig.pcache2.xFetch(pCache.pCache, pgno, 2)
	}
	if pPage != nil {
		pPgHdr = (PgHdr *)(pPage.pExtra)
		if pPgHdr.pPage != nil {
			memset(pPgHdr, 0, sizeof(PgHdr))
			pPgHdr.pPage = pPage
			pPgHdr.pData = pPage.pBuf
			pPgHdr.pExtra = (void *)&pPgHdr[1]
			memset(pPgHdr.pExtra, 0, pCache.ExtraBytes)
			pPgHdr.pCache = pCache
			pPgHdr.pgno = pgno
		}
		assert( pPgHdr.pCache == pCache )
		assert( pPgHdr.pgno == pgno )
		assert( pPgHdr.pData == pPage.pBuf )
		assert( pPgHdr.pExtra == (void *)(&pPgHdr[1]) )

		if pPgHdr.nRef == 0 {
			pCache.PageSize
		}
		pPgHdr.PageSize
		if pgno == 1 {
			pCache.pPage1 = pPgHdr
		}
	}
	page = pPgHdr
	if page == nil && eCreate {
		rc = SQLITE_NOMEM
	} else {
		rc = SQLITE_OK
	}
  return
}

//	Make every page in the cache clean.
func (p *PCache) CleanAll() {
	for page := p.pDirty; page != nil; page = p.pDirty {
		sqlite3PcacheMakeClean(page)
	}
}

//	Clear the PGHDR_NEED_SYNC flag from all dirty pages.
func (p *PCache) ClearSyncFlags() {
	for page := p.pDirty; page != nil; p = page.pDirtyNext {
		page.flags &= ~PGHDR_NEED_SYNC
	}
	p.pSynced = p.pDirtyTail
}

//	Drop every cache entry whose page number is greater than "pgno". The caller must ensure that there are no outstanding references to any pages other than page 1 with a page number greater than pgno.
//	If there is a reference to page 1 and the pgno parameter passed to this function is 0, then the data area associated with page 1 is zeroed, but the page object is not dropped.
func (p *PCache) Truncate(pgno PageNumber) {
	if p.pCache != nil {
		for page := p.pDirty; page != nil; {
			Next := page.pDirtyNext
			//	This routine never gets call with a positive pgno except right after PCache::CleanAll(). So if there are dirty pages, it must be that pgno == 0.
			assert( page.pgno > 0 )
			if p.pgno > pgno {
				assert( page.flags & PGHDR_DIRTY )
				sqlite3PcacheMakeClean(page)
			}
			page = Next
		}
		if pgno == 0 && p.pPage1 {
			memset(p.pPage1.pData, 0, p.PageSize)
			pgno = 1
		}
		sqlite3GlobalConfig.pcache2.xTruncate(p.pCache, pgno + 1)
	}
}

//	Close a cache.
func (p *PCache) Close() {
	if p.pCache {
		sqlite3GlobalConfig.pcache2.xDestroy(p.pCache)
	}
}

//	Discard the contents of the cache.
func (p *PCache) Clear() {
	p.Truncate(0)
}

//	Return a list of all dirty pages in the cache, sorted by page number.
func (p *PCache) DirtyList() *PgHdr {
	for page := p.pDirty; page != nil; page = page.pDirtyNext {
		page.pDirty = page.pDirtyNext
	}
	return pcacheSortDirtyList(p.pDirty)
}

//	Return the total number of referenced pages held by the cache.
func (p *PCache) ReferenceCount() int {
	return p.nRef
}

//	Return the total number of pages in the cache.
func (p *PCache) Pagecount() (count int) {
	if p.pCache != nil {
		count = sqlite3GlobalConfig.pcache2.xPagecount(p.pCache)
	}
	return nPage
}

//	Set the suggested cache-size value.
func (p *PCache) SetCachesize(size int) {
	p.size = size
	if p.pCache != nil {
		sqlite3GlobalConfig.pcache2.xCachesize(p.pCache, p.NumberOfCachePages())
	}
}

//	Free up as much memory as possible from the page cache.
func (p *PCache) Shrink() {
	if p.pCache {
		sqlite3GlobalConfig.pcache2.xShrink(p.pCache)
	}
}

//	For all dirty pages currently in the cache, invoke the specified callback. This is only used if the SQLITE_CHECK_PAGES macro is defined.
func (p *PCache) IterateDirty(f func(* PgHdr)) {
	for dirty := p.pDirty; dirty != nil; dirty = dirty.pDirtyNext {
		f(dirty)
	}
}