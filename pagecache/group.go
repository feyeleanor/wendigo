//	Each page cache (or PCache) belongs to a Group. A Group is a set of one or more PCaches that are able to recycle each others unpinned pages when they are under memory pressure. A Group is an instance of the following object.
//	There is a single global Group that all PCaches are a member of. This requires a mutex in order to be threadsafe.
//	There is only a single Group which is the pcache1.grp global variable and its mutex is SQLITE_MUTEX_STATIC_LRU.
type Group struct {
	mutex					*sqlite3_mutex			//	MUTEX_STATIC_LRU or NULL
	MaxPage					uint					//	Sum of nMax for purgeable caches
	MinPage					uint					//	Sum of nMin for purgeable caches
	Pinned					uint					//	nMaxpage + 10 - MinPage
	CurrentPage				uint					//	Number of purgeable pages allocated
	LruHead, LruTail		*PgHdr1					//	LRU list of unpinned pages
}

//	If there are currently more than MaxPage pages allocated, try to recycle pages to reduce the number allocated to MaxPage.
func (g *Group) EnforceMaxPage() {
	for g.CurrentPage > g.MaxPage && g.LruTail {
		p := g.LruTail
		assert( p.pCache.Group == g )
		pcache1PinPage(p)
		pcache1RemoveFromHash(p)
		pcache1FreePage(p)
	}
}
