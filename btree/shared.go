//	An instance of this object represents a single database file.
//
//	A single database file can be in use at the same time by two or more database connections. When two or more connections are sharing the same database file, each connection has it own
//	private Btree object for the file and each of those Btrees points to this one BtShared object. BtShared.nRef is the number of connections currently sharing this database file.
//
//	Fields in this structure are accessed under the BtShared.mutex mutex, except for nRef and Next which are accessed under the global SQLITE_MUTEX_STATIC_MASTER mutex. The pPager field
//	may not be modified once it is initially set as long as nRef>0. The Schema field may be set once under BtShared.mutex and thereafter is unchanged as long as nRef>0.
//
//	isPending:
//
//		If a BtShared client fails to obtain a write-lock on a database table (because there exists one or more read-locks on the table), the shared-cache enters 'pending-lock'
//		state and isPending is set to true.
//
//		The shared-cache leaves the 'pending lock' state when either of the following occur:
//
//			1) The current writer (BtShared.pWriter) concludes its transaction, OR
//			2) The number of locks held by other connections drops to zero.
//
//		while in the 'pending-lock' state, no connection may start a new transaction.
//
//	This feature is included to help prevent writer-starvation.

type BtShared struct {
	pPager			*Pager				//	The page cache
	db				*sqlite3			//	Database connection currently using this Btree
	pCursor			Cursor				//	A list of all open cursors
	pPage1			*MemoryPage			//	First page of the database
	openFlags		byte				//	Flags to sqlite3BtreeOpen()
	autoVacuum		bool				//	True if auto-vacuum is enabled
	incrVacuum		bool				//	True if incr-vacuum is enabled
	inTransaction	bool				//	Transaction state
	max1bytePayload	byte				//	Maximum first byte of cell for a 1-byte payload
	Flags			uint16				//	Boolean parameters.  See BTS_* macros below
	maxLocal		uint16				//	Maximum local payload in non-LEAFDATA tables
	minLocal		uint16				//	Minimum local payload in non-LEAFDATA tables
	maxLeaf			uint16				//	Maximum local payload in a LEAFDATA table
	minLeaf			uint16				//	Minimum local payload in a LEAFDATA table
	pageSize		int					//	Total number of bytes on a page
	usableSize		int					//	Number of usable bytes on each page
	nTransaction	int					//	Number of open transactions (read + write)
	nPage			int					//	Number of pages in the database
	*Schema								//	Pointer to space allocated by Btree::Schema()
	xFreeSchema		func()				//	Destructor for BtShared.Schema */
	mutex			*sqlite3_mutex		//	Non-recursive mutex required to access this object
	pHasContent		*Bitvec				//	Set of pages moved to free-list this transaction
	nRef			int					//	Number of references to this structure
	Next			*BtShared			//	Next on a list of sharable BtShared structs
	*Lock								//	List of locks held on this shared-btree struct
	pWriter			*Btree				//	Btree with currently open write transaction
	pTmpSpace		[]byte				//	BtShared.pageSize bytes of space for tmp use
}

//	Allowed values for BtShared.Flags
const(
	BTS_READ_ONLY			= 0x0001	//	Underlying file is readonly
	BTS_PAGESIZE_FIXED		= 0x0002	//	Page size can no longer be changed
	BTS_SECURE_DELETE		= 0x0004	//	PRAGMA secure_delete is enabled
	BTS_INITIALLY_EMPTY		= 0x0008	//	Database was empty at trans start
	BTS_NO_WAL				= 0x0010	//	Do not open write-ahead-log files
	BTS_EXCLUSIVE 			= 0x0020	//	pWriter has an exclusive lock
	BTS_PENDING				= 0x0040	//	Waiting for read-locks to clear
)

//	Get a page from the pager. Initialize the MemoryPage.pBt and MemoryPage.aData elements if needed.
//
//	If the noContent flag is set, it means that we do not care about the content of the page at this time. So do not go to the disk
//	to fetch the content. Just fill in the content with zeros for now. If in the future we call DbPage::Write() on this page, that
//	means we have started to be concerned about content and the disk read should occur at that point.

func (pBt *BtShared) GetPage(pgno PageNumber, noContent bool) (ppPage MemoryPage, rc int) {
	pDbPage		*DbPage

	if pDbPage, rc = pBt.pPager.Acquire(pgno, noContent); rc != SQLITE_OK {
		return
	}
	ppPage = pDbPage.BtreePage(pgno, pBt)
	rc = SQLITE_OK
	return
}

//	Get a page from the pager and initialize it. This routine is just a convenience wrapper around separate calls to GetPage() and Initialize().
//	If an error occurs, then the value *ppPage is set to is undefined. It may remain unchanged, or it may be set to an invalid value.
func (pBt *BtShared) GetPageAndInitialize(pgno PageNumber) (ppPage *MemoryPage, rc int) {
	if pgno > btreePagecount(pBt) {
		rc = SQLITE_CORRUPT_BKPT
	} else {
		if ppPage, rc = pBt.GetPage(pgno, false); rc == SQLITE_OK {
			if rc = ppPage.Initialize(); rc != SQLITE_OK {
				ppPage.Release()
			}
    	}
	}
	assert( pgno != 0 || rc == SQLITE_CORRUPT )
	return
}
