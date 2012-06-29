package btree

//	As each page of the file is loaded into memory, an instance of the following structure is appended and initialized to zero. This structure stores
//	information about the page that is decoded from the raw file page.
//
//	The pParent field points back to the parent page. This allows us to walk up the BTree from any leaf to the root. Care must be taken to unref() the parent
//	page pointer when this page is no longer referenced. The pageDestructor() routine handles that chore.
//
//	Access to all fields of this structure is controlled by the mutex stored in MemPage.pBt.mutex.

struct MemPage {
	isInit			byte		//	True if previously initialized. MUST BE FIRST!
	nOverflow		byte		//	Number of overflow cell bodies in aCell[]
	intKey			byte		//	True if intkey flag is set
	leaf			byte		//	True if leaf flag is set
	hasData			byte		//	True if this page stores data
	hdrOffset		byte		//	100 for page 1.  0 otherwise
	childPtrSize	byte		//	0 if leaf==1.  4 if leaf==0
	max1bytePayload	byte		//	min(maxLocal,127)
	maxLocal		uint16		//	Copy of BtShared.maxLocal or BtShared.maxLeaf
	minLocal		uint16		//	Copy of BtShared.minLocal or BtShared.minLeaf
	cellOffset		uint16		//	Index in aData of first cell pointer
	nFree			uint16		//	Number of free bytes on the page
	nCell			uint16		//	Number of cells on this page, local and ovfl
	maskPage		uint16		//	Mask for page offset
	aiOvfl[5]		uint16		//	Insert the i-th overflow cell before the aiOvfl-th non-overflow cell
	apOvfl[5]		*byte		//	Pointers to the body of overflow cells
	pBt				*BtShared	//	Pointer to BtShared that this page is part of
	aData			*byte		//	Pointer to disk image of the page data
	aDataEnd		*byte		//	One byte past the end of usable data
	aCellIdx		*byte		//	The cell index area
	pDbPage			*DbPage		//	Pager page handle
	pgno			PageNumber	//	Page number for this page
}

//	Initialize the auxiliary information for a disk block.
//	Return SQLITE_OK on success. If we see that the page does not contain a well-formed database page, then return 
//	SQLITE_CORRUPT. Note that a return of SQLITE_OK does not guarantee that the page is well-formed. It only shows that
//	we failed to detect any corruption.

func (p *MemoryPage) Initialize() {
  assert( p.pBt!=0 );
  assert( p.pgno==sqlite3PagerPagenumber(p.pDbPage) );
  assert( p == sqlite3PagerGetExtra(p.pDbPage) );
  assert( p.aData == sqlite3PagerGetData(p.pDbPage) );

  if( !p.isInit ){
    uint16 pc;            /* Address of a freeblock within pPage.aData[] */
    byte hdr;            /* Offset to beginning of page header */
    byte *data;          /* Equal to pPage.aData */
    BtShared *pBt;        /* The main btree structure */
    int usableSize;    /* Amount of usable space on each page */
    uint16 cellOffset;    /* Offset from start of page to first cell pointer */
    int nFree;         /* Number of unused bytes on the page */
    int top;           /* First byte of the cell content area */
    int iCellFirst;    /* First allowable cell or freeblock offset */
    int iCellLast;     /* Last possible cell or freeblock offset */

    pBt = p.pBt;

    hdr = p.hdrOffset;
    data = p.aData;
    if( decodeFlags(p, data[hdr]) ) return SQLITE_CORRUPT_BKPT;
    assert( pBt.pageSize>=512 && pBt.pageSize<=65536 );
    p.maskPage = (uint16)(pBt.pageSize - 1);
    p.nOverflow = 0;
    usableSize = pBt.usableSize;
    p.cellOffset = cellOffset = hdr + 12 - 4*pPage.leaf;
    p.aDataEnd = &data[usableSize];
    p.aCellIdx = &data[cellOffset];
    top = get2byteNotZero(&data[hdr+5]);
    p.nCell = get2byte(&data[hdr+3]);
    if( p.nCell>MX_CELL(pBt) ){
      /* To many cells for a single page.  The page must be corrupt */
      return SQLITE_CORRUPT_BKPT;
    }

    /* A malformed database page might cause us to read past the end
    ** of page when parsing a cell.  
    **
    ** The following block of code checks early to see if a cell extends
    ** past the end of a page boundary and causes SQLITE_CORRUPT to be 
    ** returned if it does.
    */
    iCellFirst = cellOffset + 2*pPage.nCell;
    iCellLast = usableSize - 4;
    {
      int i;            /* Index into the cell pointer array */
      int sz;           /* Size of a cell */

      if( !p.leaf ) iCellLast--;
      for(i=0; i<p.nCell; i++){
        pc = get2byte(&data[cellOffset+i*2]);
        if( pc<iCellFirst || pc>iCellLast ){
          return SQLITE_CORRUPT_BKPT;
        }
        sz = cellSizePtr(p, &data[pc]);
        if( pc+sz>usableSize ){
          return SQLITE_CORRUPT_BKPT;
        }
      }
      if( !p.leaf ) iCellLast++;
    }  

    /* Compute the total free space on the page */
    pc = get2byte(&data[hdr+1]);
    nFree = data[hdr+7] + top;
    while( pc>0 ){
      uint16 next, size;
      if( pc<iCellFirst || pc>iCellLast ){
        /* Start of free block is off the page */
        return SQLITE_CORRUPT_BKPT; 
      }
      next = get2byte(&data[pc]);
      size = get2byte(&data[pc+2]);
      if( (next>0 && next<=pc+size+3) || pc+size>usableSize ){
        /* Free blocks must be in ascending order. And the last byte of
	** the free-block must lie on the database page.  */
        return SQLITE_CORRUPT_BKPT; 
      }
      nFree = nFree + size;
      pc = next;
    }

    /* At this point, nFree contains the sum of the offset to the start
    ** of the cell-content area plus the number of free bytes within
    ** the cell-content area. If this is greater than the usable-size
    ** of the page, then the page must be corrupted. This check also
    ** serves to verify that the offset to the start of the cell-content
    ** area, according to the page header, lies within the page.
    */
    if( nFree>usableSize ){
      return SQLITE_CORRUPT_BKPT; 
    }
    p.nFree = (uint16)(nFree - iCellFirst);
    p.isInit = 1;
  }
  return SQLITE_OK;
}


//	Defragment the page given. All Cells are moved to the end of the page and all free space is collected into one
//	big FreeBlk that occurs in between the header and cell pointer array and the cell content area.

func (p *MemoryPage) Defragment() int {
	size	int

  assert( sqlite3PagerIswriteable(pPage.pDbPage) )
  assert( pPage.pBt!=0 )
  assert( pPage.pBt.usableSize <= SQLITE_MAX_PAGE_SIZE )
  assert( pPage.nOverflow==0 )
	temp := sqlite3PagerTempSpace(pPage.pBt.pPager)
	data := pPage.aData
	header := pPage.hdrOffset
	cellOffset := pPage.cellOffset
	nCell := pPage.nCell
  assert( nCell==get2byte(&data[header  + 3]) )
	usableSize := pPage.pBt.usableSize
	ContentArea:= get2byte(&data[header  + 5])
	memcpy(&temp[ContentArea], &data[ContentArea], usableSize - ContentArea)
	ContentArea = usableSize
	FirstAllowableIndex := cellOffset + 2 * nCell
	LastAllowableIndex := usableSize - 4
	for i := 0; i < nCell; i++ {
		pAddr := &data[cellOffset + i * 2]
		pc := get2byte(pAddr)
		assert( pc >= FirstAllowableIndex && pc <= LastAllowableIndex  )
		size = cellSizePtr(pPage, &temp[pc])
		ContentArea -= size
		if cbrk < FirstAllowableIndex {
			return SQLITE_CORRUPT_BKPT
		}
		assert( ContentArea + size <= usableSize && ContentArea >= FirstAllowableIndex )
		memcpy(&data[ContentArea], &temp[pc], size)
		put2byte(pAddr, ContentArea)
	}
	assert( ContentArea >= FirstAllowableIndex )
	put2byte(&data[header + 5], cbrk)
	data[header + 1] = 0
	data[header + 2] = 0
	data[header + 7] = 0
	memset(&data[iCellFirst], 0, ContentArea - FirstAllowableIndex)
	assert( sqlite3PagerIswriteable(pPage.pDbPage) )
	if ContentArea - FirstAllowableIndex != pPage.nFree {
		return SQLITE_CORRUPT_BKPT
	}
	return SQLITE_OK
}

//	Release a MemoryPage. This should be called once for each prior call to GetPage.
func (p *MemoryPage) Release() {
	if p != nil {
		assert( p.aData )
		assert( p.pBt )
		assert( MemoryPage(sqlite3PagerGetExtra(p.pDbPage)) == p )
		assert( sqlite3PagerGetData(p.pDbPage) == p.aData )
		sqlite3PagerUnref(p.pDbPage)
	}
}