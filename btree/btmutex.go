//	This file contains code used to implement mutexes on Btree objects.

//	This file implements a external (disk-based) database using BTrees.
//	For a detailed discussion of BTrees, refer to
//
//		Donald E. Knuth, THE ART OF COMPUTER PROGRAMMING, Volume 3:
//		"Sorting And Searching", pages 473-480. Addison-Wesley
//		Publishing Company, Reading, Massachusetts.
//
//	The basic idea is that each page of the file contains N database entries and N+1 pointers to subpages.
//
//		----------------------------------------------------------------
//		|  Ptr(0) | Key(0) | Ptr(1) | Key(1) | ... | Key(N-1) | Ptr(N) |
//		----------------------------------------------------------------
//
//	All of the keys on the page that Ptr(0) points to have values less than Key(0). All of the keys on page Ptr(1) and its subpages have
//	values greater than Key(0) and less than Key(1). All of the keys on Ptr(N) and its subpages have values greater than Key(N-1). And so forth.
//
//	Finding a particular key requires reading O(log(M)) pages from the disk where M is the number of entries in the tree.
//
//	In this implementation, a single file can hold one or more separate BTrees. Each BTree is identified by the index of its root page.
//	The key and data for any entry are combined to form the "payload". A fixed amount of payload can be carried directly on the database
//	page. If the payload is larger than the preset amount then surplus bytes are stored on overflow pages. The payload for an entry
//	and the preceding pointer are combined to form a "Cell". Each page has a small header which contains the Ptr(N) pointer and other
//	information such as the size of key and data.
//
//	FORMAT DETAILS
//
//	The file is divided into pages. The first page is called page 1, the second is page 2, and so forth. A page number of zero indicates
//	"no such page". The page size can be any power of 2 between 512 and 65536. Each page can be either a btree page, a freelist page, an
//	overflow page, or a pointer-map page.
//
//	The first page is always a btree page. The first 100 bytes of the first page contain a special header (the "file header") that describes
//	the file. The format of the file header is as follows:
//
//		OFFSET   SIZE    DESCRIPTION
//		   0      16     Header string: "SQLite format 3\000"
//		  16       2     Page size in bytes.
//		  18       1     File format write version
//		  19       1     File format read version
//		  20       1     Bytes of unused space at the end of each page
//		  21       1     Max embedded payload fraction
//		  22       1     Min embedded payload fraction
//		  23       1     Min leaf payload fraction
//		  24       4     File change counter
//		  28       4     Reserved for future use
//		  32       4     First freelist page
//		  36       4     Number of freelist pages in the file
//		  40      60     15 4-byte meta values passed to higher layers
//
//		  40       4     Schema cookie
//		  44       4     File format of schema layer
//		  48       4     Size of page cache
//		  52       4     Largest root-page (auto/incr_vacuum)
//		  56       4     1=UTF-8
//		  60       4     User version
//		  64       4     Incremental vacuum mode
//		  68       4     unused
//		  72       4     unused
//		  76       4     unused
//
//	All of the integer values are big-endian (most significant byte first).
//	The file change counter is incremented when the database is changed
//	This counter allows other processes to know when the file has changed and thus when they need to flush their cache.
//
//	The max embedded payload fraction is the amount of the total usable space in a page that can be consumed by a single cell for standard
//	B-tree (non-LEAFDATA) tables. A value of 255 means 100%. The default is to limit the maximum cell size so that at least 4 cells will fit
//	on one page. Thus the default max embedded payload fraction is 64.
//
//	If the payload for a cell is larger than the max payload, then extra payload is spilled to overflow pages. Once an overflow page is allocated,
//	as many bytes as possible are moved into the overflow pages without letting the cell size drop below the min embedded payload fraction.
//
//	The min leaf payload fraction is like the min embedded payload fraction except that it applies to leaf nodes in a LEAFDATA tree. The maximum
//	payload fraction for a LEAFDATA tree is always 100% (or 255) and it not specified in the header.
//
//	Each btree pages is divided into three sections: The header, the cell pointer array, and the cell content area. Page 1 also has a 100-byte
//	file header that occurs before the page header.
//
//		|----------------|
//		| file header    |   100 bytes.  Page 1 only.
//		|----------------|
//		| page header    |   8 bytes for leaves.  12 bytes for interior nodes
//		|----------------|
//		| cell pointer   |   |  2 bytes per cell.  Sorted order.
//		| array          |   |  Grows downward
//		|                |   v
//		|----------------|
//		| unallocated    |
//		| space          |
//		|----------------|   ^  Grows upwards
//		| cell content   |   |  Arbitrary order interspersed with freeblocks.
//		| area           |   |  and free space fragments.
//		|----------------|
//
//	The page headers looks like this:
//
//		OFFSET   SIZE     DESCRIPTION
//		   0       1      Flags. 1: intkey, 2: zerodata, 4: leafdata, 8: leaf
//		   1       2      byte offset to the first freeblock
//		   3       2      number of cells on this page
//		   5       2      first byte of the cell content area
//		   7       1      number of fragmented free bytes
//		   8       4      Right child (the Ptr(N) value).  Omitted on leaves.
//
//	The flags define the format of this btree page. The leaf flag means that this page has no children. The zerodata flag means that this page carries
//	only keys and no data. The intkey flag means that the key is a integer which is stored in the key size entry of the cell header rather than in
//	the payload area.
//
//	The cell pointer array begins on the first byte after the page header.
//	The cell pointer array contains zero or more 2-byte numbers which are offsets from the beginning of the page to the cell content in the cell
//	content area. The cell pointers occur in sorted order. The system strives to keep free space after the last cell pointer so that new cells can
//	be easily added without having to defragment the page.
//
//	Cell content is stored at the very end of the page and grows toward the beginning of the page.
//
//	Unused space within the cell content area is collected into a linked list of freeblocks. Each freeblock is at least 4 bytes in size. The byte offset
//	to the first freeblock is given in the header. Freeblocks occur in increasing order. Because a freeblock must be at least 4 bytes in size,
//	any group of 3 or fewer unused bytes in the cell content area cannot exist on the freeblock chain. A group of 3 or fewer free bytes is called
//	a fragment. The total number of bytes in all fragments is recorded in the page header at offset 7.
//
//		SIZE    DESCRIPTION
//		  2     Byte offset of the next freeblock
//		  2     Bytes in this freeblock
//
//	Cells are of variable length. Cells are stored in the cell content area at the end of the page. Pointers to the cells are in the cell pointer array
//	that immediately follows the page header. Cells is not necessarily contiguous or in order, but cell pointers are contiguous and in order.
//
//	Cell content makes use of variable length integers. A variable length integer is 1 to 9 bytes where the lower 7 bits of each byte are used. The integer
//	consists of all bytes that have bit 8 set and the first byte with bit 8 clear. The most significant byte of the integer appears first. A variable-length
//	integer may not be more than 9 bytes long.
//	As a special case, all 8 bytes of the 9th byte are used as data. This allows a 64-bit integer to be encoded in 9 bytes.
//
//		0x00                      becomes  0x00000000
//		0x7f                      becomes  0x0000007f
//		0x81 0x00                 becomes  0x00000080
//		0x82 0x00                 becomes  0x00000100
//		0x80 0x7f                 becomes  0x0000007f
//		0x8a 0x91 0xd1 0xac 0x78  becomes  0x12345678
//		0x81 0x81 0x81 0x81 0x01  becomes  0x10204081
//
//	Variable length integers are used for rowids and to hold the number of bytes of key and data in a btree cell.
//
//	The content of a cell looks like this:
//
//		SIZE    DESCRIPTION
//		  4     Page number of the left child. Omitted if leaf flag is set.
//		 var    Number of bytes of data. Omitted if the zerodata flag is set.
//		 var    Number of bytes of key. Or the key itself if intkey flag is set.
//		  *     Payload
//		  4     First page of the overflow chain.  Omitted if no overflow
//
//	Overflow pages form a linked list. Each page except the last is completely filled with data (pagesize - 4 bytes). The last page can have as little
//	as 1 byte of data.
//
//		SIZE    DESCRIPTION
//		  4     Page number of next overflow page
//		  *     Data
//
//	Freelist pages come in two subtypes: trunk pages and leaf pages. The file header points to the first in a linked list of trunk page. Each trunk page
//	points to multiple leaf pages. The content of a leaf page is unspecified. A trunk page looks like this:
//
//		SIZE    DESCRIPTION
//		  4     Page number of next trunk page
//		  4     Number of leaf pointers on this page
//		  *     zero or more pages numbers of leaves

//	The following value is the maximum cell size assuming a maximum page size give above.
#define MX_CELL_SIZE(pBt)  ((int)(pBt.pageSize-8))

//	The maximum number of cells on a single page of the database. This assumes a minimum cell size of 6 bytes (4 bytes for the cell itself plus 2 bytes
//	for the index to the cell in the page header). Such small cells will be rare, but they are possible.
#define MX_CELL(pBt) ((pBt.pageSize-8)/6)

//	Page type flags. An ORed combination of these flags appear as the first byte of on-disk image of every BTree page.
const(
	PTF_INTKEY		= 0x01
	PTF_ZERODATA	= 0x02
	PTF_LEAFDATA	= 0x04
	PTF_LEAF		= 0x08
)


//	A Btree handle
//
//	A database connection contains a pointer to an instance of this object for every database file that it has open. This structure is opaque to the database connection.
//	The database connection cannot see the internals of this structure and only deals with pointers to this structure.
//
//	For some database files, the same underlying database cache might be shared MemoryPage multiple connections. In that case, each connection has it own instance of this object.
//	But each instance of this object points to the same BtShared object. The database cache and the schema associated with the database file are all contained within
//	the MemoryPaged object.
//
//	All fields in this structure are accessed under sqlite3.mutex. The pBt pointer itself may not be changed while there exists cursors in the referenced BtShared that point
//	back to this Btree since those cursors have to go through this Btree to find their BtShared and they often do so without holding sqlite3.mutex.

type Btree struct {
	db			*sqlite3			//	The database connection holding this btree
	pBt			*BtShared			//	Sharable content of this btree
	intrans		byte				//	TRANS_NONE, TRANS_READ or TRANS_WRITE
	sharable	bool				//	True if we can share pBt with another db
	locked		bool				//	True if db currently has pBt locked
	wantToLock	int					//	Number of nested calls to Lock()
	nBackup		int					//	Number of backup operations reading this btree
	Next		*Btree				//	List of other sharable Btrees from the same db
	Prev		*Btree				//	Back pointer of the same list
	Lock							//	Object used to lock page 1
}

//	Btree.inTrans may take one of the following values.
//
//	If the shared-data extension is enabled, there may be multiple users of the Btree structure. At most one of these may open a write transaction, but any number may have active read transactions.
const(
	TRANS_NONE	= iota
	TRANS_READ
	TRANS_WRITE
)

//	Maximum depth of an SQLite B-Tree structure. Any B-Tree deeper than this will be declared corrupt. This value is calculated based on a
//	maximum database size of 2^31 pages a minimum fanout of 2 for a root-node and 3 for all other internal nodes.
//
//	If a tree that appears to be taller than this is encountered, it is assumed that the database is corrupt.

const	BTCURSOR_MAX_DEPTH	= 20


//	These macros define the location of the pointer-map entry for a database page. The first argument to each is the number of usable bytes on each page of the database (often 1024). The second is the page number to look up in the pointer map.
//
//	ptrmap_ptroffset returns the offset of the requested map entry.
//
//	If the pgno argument passed to BtShared::Pageno is a pointer-map page, then pgno is returned. So (pgno == pgsz.Pageno(pgno)) can be	used to test if pgno is a pointer-map page. BtShared::IsMapPage implements this test.
func ptrmap_ptroffset(pgptrmap, pgno PageNumber) PageNumber {
	return 5 * (pgno - pgptrmap - 1)
}

func (pBt *BtShared) IsMapPage(pgno PageNumber) bool {
	return pBt.Pageno(pgno) == pgno
}

//	The pointer map is a lookup table that identifies the parent page for each child page in the database file. The parent page is the page that contains a pointer to the child. Every page in the database contains 0 or 1 parent pages. (In this context 'database page' refers to any page that is not part of the pointer map itself.) Each pointer map entry consists of a single byte 'type' and a 4 byte parent page number.
//	The identifiers below are the valid types.
//
//	The purpose of the pointer map is to facilitate moving pages from one position in the file to another as part of autovacuum. When a page is moved, the pointer in its parent must be updated to point to the new location. The pointer map is used to locate the parent page quickly.
const(
	ROOT_PAGE					= iota		//	The database page is a root-page. The page-number is not used in this case.
	FREE_PAGE								//	The database page is an unused (free) page. The page-number is not used in this case.
	FIRST_OVERFLOW_PAGE						//	The database page is the first page in a list of overflow pages. The page number identifies the page that contains the cell with a pointer to this overflow page.
	SECONDARY_OVERFLOW_PAGE					//	The database page is the second or later page in a list of overflow pages. The page-number identifies the previous page in the overflow page list.
	NON_ROOT_BTREE_PAGE						//	The database page is a non-root btree page. The page number identifies the parent page in the btree.
)

//	A bunch of assert() statements to check the transaction state variables of handle p (type Btree*) are internally consistent.
func (p *Btree) EnforceIntegrity() {
	assert(p.pBt.inTransaction != TRANS_NONE || p.pBt.nTransaction == 0)
	assert(p.pBt.inTransaction >= p.inTrans)
}


//	Obtain the BtShared mutex associated with B-Tree handle p. Also, set BtShared.db to the database handle associated with p and the p.locked boolean to true.
func (p *Btree) Lock() {
	assert(!p.locked)
	p.pBt.mutex.Lock()
	p.pBt.db = p.db
	p.locked = true
}

//	Release the BtShared mutex associated with B-Tree handle p and clear the p.locked boolean.
func (p *Btree) Unlock() {
	assert(p.Locked)
	assert(p.db == p.pBt.db)
	p.pBt.mutex.Unlock()
	p.locked = false
}

//	Enter a mutex on the given BTree object.
//
//	If the object is not sharable, then no mutex is ever required and this routine is a no-op. The underlying mutex is non-recursive.
//	But we keep a reference count in Btree.wantToLock so the behavior of this interface is recursive.
//
//	To avoid deadlocks, multiple Btrees are locked in the same order by all database connections. The p.Next is a list of other
//	Btrees belonging to the same database connection as the p Btree which need to be locked after p. If we cannot get a lock on
//	p, then first unlock all of the others on p.Next, then wait for the lock to become available on p, then relock all of the
//	subsequent Btrees that desire a lock.

func (p *Btree) Lock() {
	pLater	*Btree

	//	Some basic sanity checking on the Btree. The list of Btrees connected by Next and pPrev should be in sorted order by
	//	Btree.pBt value. All elements of the list should belong to the same connection. Only shared Btrees are on the list.
	assert(p.Next == 0 || p.Next.pBt > p.pBt)
	assert(p.pPrev == 0 || p.pPrev.pBt < p.pBt)
	assert(p.Next == 0 || p.Next.db == p.db)
	assert(p.pPrev == 0 || p.pPrev.db == p.db)
	assert(p.sharable || (p.Next == 0 && p.pPrev == 0))

	//	Check for locking consistency
	assert(!p.locked || p.wantToLock > 0)
	assert(p.sharable || p.wantToLock == 0)

	//	We should already hold a lock on the database connection

	//	Unless the database is sharable and unlocked, then BtShared.db should already be set correctly.
	assert((!p.locked && p.sharable) || p.pBt.db == p.db)

	if !p.sharable {
		return
	}
	p.wantToLock++
	if p.locked {
		return
	}

	//	In most cases, we should be able to acquire the lock we want without having to go throught the ascending lock procedure that follows. Just be sure not to block.
	if p.pBt.mutex.TestLock() == SQLITE_OK {
		p.pBt.db = p.db
		p.locked = true
		return
	}

	//	To avoid deadlock, first release all locks with a larger BtShared address. Then acquire our lock. Then reacquire the other BtShared locks that we used to hold in ascending order.
	for pLater = p.Next; pLater; pLater = pLater.Next {
		assert(pLater.sharable)
		assert(pLater.Next == 0 || pLater.Next.pBt > pLater.pBt)
		assert(!pLater.locked || pLater.wantToLock > 0)
		if pLater.locked {
			pLater.Unlock()
		}
	}
	p.Lock()

	for pLater = p.Next; pLater; pLater = pLater.Next {
		if pLater.wantToLock {
			pLater.Lock()
		}
	}
}

//	Exit the recursive mutex on a Btree.
func (p *Btree) Unlock() {
	if p.sharable {
		assert(p.wantToLock > 0)
		p.wantToLock--
		if p.wantToLock == 0 {
			p.Unlock()
		}
	}
}

func (p *Btree) CriticalSection(f func()) {
	defer p.Unlock()
	p.Lock()
	f()
}

//	Enter and leave a mutex on a Btree given a cursor owned by that Btree. These entry points are used by incremental I/O and can be
//	omitted if that module is not used.
func (pCur *Cursor) Lock() {
	pCur.pBtree.Lock()
}

func (p *Cursor) Unlock() {
	pCur.pBtree.Unlock()
}

func (p *Cursor) CriticalSection(f func()) {
	defer p.Unlock()
	Lock()
	f()
}

//	Enter the mutex on every Btree associated with a database connection. This is needed (for example) prior to parsing a statement since we will be comparing
//	table and column names against all schemas and we do not want those schemas being reset out from under us.
//
//	There is a corresponding leave-all procedures.
//
//	Enter the mutexes in accending order by BtShared pointer address to avoid the possibility of deadlock when two threads with two or more btrees in common both
//	try to lock all their btrees at the same instant.
func (db *sqlite3) LockAll() {
	for _, database := range db.Databases {
		if database.pBt != nil {
			database.pBt.Lock()
		}
	}
}

func (db *sqlite3) LeaveBtreeAll() {
	for _, database := range db.Databases {
		if database.pBt != nil {
			database.pBt.Unlock()
		}
	}
}

//	Return true if a particular Btree requires a lock. Return FALSE if no lock is ever required since it is not sharable.
func (p *Btree) BtreeShareable() {
	return p.sharable
}
