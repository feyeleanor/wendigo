package btree

//	The pParent field points back to the parent page. This allows us to walk up the BTree from any IsLeaf to the root. Care must be taken to unref() the parent page pointer when this page is no longer referenced. The pageDestructor() routine handles that chore.
//
//	Access to all fields of this structure is controlled by the mutex stored in MemoryPage.BtShared.mutex.


/*
 * A MemoryPage maintains metadata for a page loaded into memory from a binary store. The metadata is decoded from the raw binary page.
 */
type MemoryPage struct {
//struct MemPage {
	isInit			bool		//	True if previously initialized. MUST BE FIRST!
	nOverflow		byte		//	Number of overflow cell bodies in aCell[]
	IsIntegerKey	bool		//	True if IsIntegerKey flag is set
	IsLeaf			bool		//	True if IsLeaf flag is set
	HasData			bool		//	True if this page stores data
	hdrOffset		byte		//	100 for page 1.  0 otherwise
	childPtrSize	byte		//	0 if IsLeaf.  4 if !IsLeaf
	max1bytePayload	byte		//	min(maxLocal,127)
	maxLocal		uint16		//	Copy of BtShared.maxLocal or BtShared.maxIsLeaf
	minLocal		uint16		//	Copy of BtShared.minLocal or BtShared.minIsLeaf
	cellOffset		uint16		//	Index in aData of first cell pointer
	nFree			uint16		//	Number of free bytes on the page

	maskPage		uint16		//	Mask for page offset
	aiOvfl[5]		uint16		//	Insert the i-th overflow cell before the aiOvfl-th non-overflow cell
	apOvfl[5]		*byte		//	Pointers to the body of overflow cells
	*BtShared
	aData			*byte		//	Pointer to disk image of the page data
	aDataEnd		*byte		//	One byte past the end of usable data

	nCell			uint16		//	Number of cells on this page, local and ovfl
	CellIndices		[]byte

	*DbPage						//	Pager page handle
	pgno			PageNumber	//	Page number for this page
}

//	Initialize the auxiliary information for a disk block.
//	Return SQLITE_OK on success. If we see that the page does not contain a well-formed database page, then return
//	SQLITE_CORRUPT. Note that a return of SQLITE_OK does not guarantee that the page is well-formed. It only shows that
//	we failed to detect any corruption.

func (p *MemoryPage) Initialize() (rc int) {
	assert( p.BtShared != nil )
	assert( p == p.GetExtra() )
	assert( &(p.aData[0]) == &(p.GetData())[0] )

	if !p.isInit {
		pBt := p.BtShared
		hdr := p.hdrOffset
		data := Buffer(p.aData)

		if !decodeFlags(p, data[hdr]) {
			assert( pBt.pageSize >= 512 && pBt.pageSize <= 65536 )
			p.maskPage = uint16(pBt.pageSize - 1)
			p.nOverflow = 0
			usableSize := pBt.usableSize
			cellOffset := hdr + 12
			if pPage.IsLeaf {
				cellOffset -= 4
			}
			p.cellOffset = cellOffset
			p.aDataEnd = &data[usableSize]
			p.CellIndices = &data[cellOffset]
			top := data[hdr + 5].ReadCompressedIntNotZero()		//	First byte of the cell content area
			if p.nCell = data[hdr + 3].ReadUint16(); p.nCell > MX_CELL(pBt) {
				//	Too many cells for a single page. The page must be corrupt
				return SQLITE_CORRUPT_BKPT
			}

			//	A malformed database page might cause us to read past the end of page when parsing a cell.
			//	The following block of code checks early to see if a cell extends past the end of a page boundary and causes SQLITE_CORRUPT to be returned if it does.
			iCellFirst := cellOffset + 2 * pPage.nCell		//	First allowable cell or freeblock offset
			iCellLast := usableSize - 4						//	Last possible cell or freeblock offset

			if !p.IsLeaf {
				iCellLast--
			}
			for i := 0; i < p.nCell; i++ {
				pc := data[cellOffset + (i * 2)].ReadUint16()
				if pc < iCellFirst || pc > iCellLast {
					return SQLITE_CORRUPT_BKPT
				}
				if sz := p.cellSize(&data[pc]); pc + sz > usableSize {
					return SQLITE_CORRUPT_BKPT
				}
			}
			if !p.IsLeaf {
				iCellLast++
			}

			//	Compute the total free space on the page
			pc := data[hdr + 1].ReadUint16()
			nFree := data[hdr + 7] + top
	    	for pc > 0 {
				if pc < iCellFirst || pc > iCellLast {
					//	Start of free block is off the page
					return SQLITE_CORRUPT_BKPT
				}
				next, size := data[pc].ReadUint16(), data[pc + 2].ReadUint16()
				if (next > 0 && next <= pc + size + 3) || pc + size > usableSize {
					//	Free blocks must be in ascending order. And the last byte of the free-block must lie on the database page.
					return SQLITE_CORRUPT_BKPT
				}
				nFree = nFree + size
				pc = next
			}

			//	At this point, nFree contains the sum of the offset to the start of the cell-content area plus the number of free bytes within the cell-content area. If this is greater than the usable-size of the page, then the page must be corrupted. This check also serves to verify that the offset to the start of the cell-content area, according to the page header, lies within the page.
			if nFree > usableSize {
				return SQLITE_CORRUPT_BKPT
			}
			p.nFree = uint16(nFree - iCellFirst)
			p.isInit = true
		} else {
			rc = SQLITE_CORRUPT_BKPT
		}
	}
	return
}


//	Defragment the page given. All Cells are moved to the end of the page and all free space is collected into one
//	big FreeBlk that occurs in between the header and cell pointer array and the cell content area.

func (p *MemoryPage) Defragment() int {
	size	int

	assert( pPage.BtShared!=0 )
	assert( pPage.BtShared.usableSize <= SQLITE_MAX_PAGE_SIZE )
	assert( pPage.nOverflow==0 )
	temp := sqlite3PagerTempSpace(pPage.BtShared.pPager)
	data := Buffer(pPage.aData)
	header := pPage.hdrOffset
	cellOffset := pPage.cellOffset
	nCell := pPage.nCell
	assert( nCell == data[header + 3].ReadUint16() )
	usableSize := pPage.BtShared.usableSize
	ContentArea:= data[header + 5].ReadUint16()
	memcpy(&temp[ContentArea], &data[ContentArea], usableSize - ContentArea)
	ContentArea = usableSize
	FirstAllowableIndex := cellOffset + 2 * nCell
	LastAllowableIndex := usableSize - 4
	for i := 0; i < nCell; i++ {
		pAddr := cellOffset + (i * 2)
		pc := data[pAddr].ReadUint16()
		assert( pc >= FirstAllowableIndex && pc <= LastAllowableIndex  )
		size = pPage.cellSize(&temp[pc])
		ContentArea -= size
		if cbrk < FirstAllowableIndex {
			return SQLITE_CORRUPT_BKPT
		}
		assert( ContentArea + size <= usableSize && ContentArea >= FirstAllowableIndex )
		memcpy(&data[ContentArea], &temp[pc], size)
		data[pAddr:].WriteUint16(ContentArea)
	}
	assert( ContentArea >= FirstAllowableIndex )
	data[header + 5:].WriteUint16(cbrk)
	data[header + 1] = 0
	data[header + 2] = 0
	data[header + 7] = 0
	memset(&data[iCellFirst], 0, ContentArea - FirstAllowableIndex)
	if ContentArea - FirstAllowableIndex != pPage.nFree {
		return SQLITE_CORRUPT_BKPT
	}
	return SQLITE_OK
}

//	Release a MemoryPage. This should be called once for each prior call to GetPage.
func (p *MemoryPage) Release() {
	if p != nil {
		assert( p.aData )
		assert( p.BtShared )
		assert( MemoryPage(p.GetExtra()) == p )
		assert( p.GetData() == p.aData )
		p.DbPage.Unref()
	}
}

//	Given a btree page and a cell index (0 means the first cell on the page, 1 means the second cell, and so forth) return a pointer to the cell content.
//	This routine works only for pages that do not contain overflow cells.
func (page *MemoryPage) FindCell(cell int) Buffer {
	return Buffer(page.aData[int(page.maskPage & Buffer(page.CellIndices[2 * cell]).ReadUint16()):])
}

//	This a more complex version of FindCell() that works for pages that do contain overflow cells.
func (pPage *MemoryPage) FindOverflowCell(cell int) Buffer {
	for i := pPage.nOverflow - 1; i >= 0; i-- {
		switch k := pPage.aiOvfl[i]; {
		case k == cell:
			return Buffer(pPage.apOvfl[i])
		case k < cell:
			cell--
		}
	}
	return pPage.FindCell(cell)
}

//	This routine redistributes cells on the iParentIdx'th child of pParent (hereafter "the page") and up to 2 siblings so that all pages have about the same amount of free space. Usually a single sibling on either side of the page are used in the balancing, though both siblings might come from one side if the page is the first or last child of its parent. If the page has fewer than 2 siblings (something which can only happen if the page is a root page or a child of a root page) then all available siblings participate in the balancing.
//	The number of siblings of the page might be increased or decreased by one or two in an effort to keep pages nearly full but not over full.
//	Note that when this routine is called, some of the cells on the page might not actually be stored in MemoryPage.aData[]. This can happen if the page is overfull. This routine ensures that all cells allocated to the page and its siblings fit into MemoryPage.aData[] before returning.
//	In the course of balancing the page and its siblings, cells may be inserted into or removed from the parent page (pParent). Doing so may cause the parent page to become overfull or underfull. If this happens, it is the responsibility of the caller to invoke the correct balancing routine to fix this problem (see the Balance() routine).
//	If this routine fails for any reason, it might leave the database in a corrupted state. So if this routine fails, the database should be rolled back.
//	The third argument to this function, aOvflSpace, is a pointer to a buffer big enough to hold one page. If while inserting cells into the parent page (pParent) the parent page becomes overfull, this buffer is used to store the parent's overflow cells. Because this function inserts a maximum of four divider cells into the parent page, and the maximum size of a cell stored within an internal node is always less than 1/4 of the page-size, the aOvflSpace[] buffer is guaranteed to be large enough for all overflow cells.
//	If aOvflSpace is set to a null pointer, this function returns SQLITE_NOMEM.
func (pParent *MemoryPage) BalanceNonroot(iParentIdx int, aOvflSpace []byte, isRoot bool) (rc int) {
	int nCell = 0;               /* Number of cells in apCell[] */
	int nMaxCells = 0;           /* Allocated size of apCell, szCell, aFrom. */
	int nNew = 0;                /* Number of pages in apNew[] */
	int nOld;                    /* Number of pages in apOld[] */
	int i, j, k;                 /* Loop counters */
	int nxDiv;                   /* Next divider slot in pParent.CellIndices[] */
	uint16 IsLeafCorrection;          /* 4 if pPage is a IsLeaf.  0 if not */
	int IsLeafData;                /* True if pPage is a IsLeaf of a IsLeafDATA tree */
	int usableSpace;             /* Bytes in pPage beyond the header */
	int pageFlags;               /* Value of pPage.aData[0] */
	int subtotal;                /* Subtotal of bytes in cells on one page */
	int iSpace1 = 0;             /* First unused byte of aSpace1[] */
	int iOvflSpace = 0;          /* First unused byte of aOvflSpace[] */
	int szScratch;               /* Size of scratch memory requested */

	byte *pRight;                  /* Location in parent of right-sibling pointer */
	byte *apDiv[NB-1];             /* Divider cells in pParent */
	int cntNew[NB+2];            /* Index in aCell[] of cell after i-th page */
	int szNew[NB+2];             /* Combined size of cells place on i-th page */
	byte **apCell = 0;             /* All cells begin balanced */
	uint16 *szCell;                 /* Local size of all cells in apCell[] */
	byte *aSpace1;                 /* Space for copies of dividers cells */

	pBt := pParent.BtShared
	apOld := make([]*MemoryPage, 0, NB)				//	pPage and up to two siblings
	apCopy := make([]*MemoryPage, 0, NB)			//	Private copies of apOld[] pages
	apNew := make([]*MemoryPage, 0, NB + 2)			//	pPage and up to NB siblings after balancing

	//	At this point pParent may have at most one overflow cell. And if this overflow cell is present, it must be the cell with index iParentIdx. This scenario comes about when this function is called (indirectly) from sqlite3BtreeDelete().
	assert( pParent.nOverflow == 0 || pParent.nOverflow == 1 )
	assert( pParent.nOverflow == 0 || pParent.aiOvfl[0] == iParentIdx )

	if len(aOvflSpace) == 0 {
		return SQLITE_NOMEM
	}

	//	Find the sibling pages to balance. Also locate the cells in pParent that divide the siblings. An attempt is made to find NN siblings on either side of pPage. More siblings are taken from one side, however, if there are fewer than NN siblings on the other side. If pParent has NB or fewer children then all children of pParent are taken.
	//	This loop also drops the divider cells from the parent page. This way, the remainder of the function does not have to deal with any overflow cells in the parent page, since if any existed they will have already been removed.
	if i = pParent.nOverflow + pParent.nCell; i < 2 {
		nxDiv = 0
		nOld = i + 1
	} else {
		nOld = 3
		switch iParentIdx {
		case 0:
			nxDiv = 0
		case i:
			nxDiv = i - 2
		default:
			nxDiv = iParentIdx - 1
		}
		i = 2
	}
	if i + nxDiv - pParent.nOverflow == pParent.nCell {
		pRight = pParent.aData[pParent.hdrOffset + 8]
	} else {
		pRight = pParent.FindCell(i + nxDiv - pParent.nOverflow)
	}
	pgno := PageNumber(Buffer(pRight).ReadUint32())
	for {
		apOld[i], rc = pBt.GetPageAndInitialize(pgno)
		if rc != SQLITE_OK {
			goto balance_cleanup
		}
		nMaxCells += 1 + apOld[i].nCell + apOld[i].nOverflow
		if i--; i == 0 {
			break
		}

		if i + nxDiv == pParent.aiOvfl[0] && pParent.nOverflow {
			apDiv[i] = pParent.apOvfl[0]
			pgno = PageNumber(Buffer(apDiv[i:]).ReadUint32())
			szNew[i] = pParent.cellSize(apDiv[i])
			pParent.nOverflow = 0
		} else {
			apDiv[i] = pParent.FindCell(i + nxDiv - pParent.nOverflow)
			pgno = PageNumber(Buffer(apDiv[i:]).ReadUint32())
			szNew[i] = pParent.cellSize(apDiv[i])

			//	Drop the cell from the parent page. apDiv[i] still points to the cell within the parent, even though it has been dropped. This is safe because dropping a cell only overwrites the first four bytes of it, and this function does not need the first four bytes of the divider cell. So the pointer is safe to use later on.
			//	But not if we are in secure-delete mode. In secure-delete mode, the dropCell() routine will overwrite the entire cell with zeroes. In this case, temporarily copy the cell into the aOvflSpace[] buffer. It will be copied out again as soon as the aSpace[] buffer is allocated.
			if pBt.Flags & BTS_SECURE_DELETE {
				iOff := SQLITE_PTR_TO_INT(apDiv[i]) - SQLITE_PTR_TO_INT(pParent.aData)
				if iOff + szNew[i] > int(pBt.usableSize) {
					rc = SQLITE_CORRUPT_BKPT
					memset(apOld, 0, (i + 1) * sizeof(MemoryPage*))
					goto balance_cleanup
				} else {
					copy(aOvflSpace[iOff:], apDiv[i:i + szNew[i]])
					apDiv[i] = aOvflSpace[apDiv[i] - pParent.aData]
				}
			}
			dropCell(pParent, i + nxDiv - pParent.nOverflow, szNew[i], &rc)
		}
	}

	//	Make nMaxCells a multiple of 4 in order to preserve 8-byte alignment
	nMaxCells = (nMaxCells + 3) &~ 3

	//	Allocate space for memory structures
	k = pBt.pageSize + ROUND(sizeof(MemoryPage), 8)
	szScratch =
       nMaxCells * sizeof(byte*)                       /* apCell */
     + nMaxCells * sizeof(uint16)                       /* szCell */
     + pBt.pageSize                               /* aSpace1 */
     + k*nOld;                                     /* Page copies (apCopy) */
	if apCell = sqlite3ScratchMalloc( szScratch ); apCell == nil {
		rc = SQLITE_NOMEM
		goto balance_cleanup
	}
	szCell = (uint16*)&apCell[nMaxCells]
	aSpace1 = (byte*)&szCell[nMaxCells]
	assert( EIGHT_BYTE_ALIGNMENT(aSpace1) )

	//	Load pointers to all cells on sibling pages and the divider cells into the local apCell[] array. Make copies of the divider cells into space obtained from aSpace1[] and remove the the divider Cells from pParent.
	//	If the siblings are on IsLeaf pages, then the child pointers of the divider cells are stripped from the cells before they are copied into aSpace1[]. In this way, all cells in apCell[] are without child pointers. If siblings are not leaves, then all cell in apCell[] include child pointers. Either way, all cells in apCell[] are alike.
	//			IsLeafCorrection:		4 if pPage is a IsLeaf. 0 if pPage is not a IsLeaf.
	//			IsLeafData:			1 if pPage holds key+data and pParent holds only keys.
	IsLeafCorrection = apOld[0].IsLeaf * 4
	IsLeafData = apOld[0].HasData
	for i = 0; i < nOld; i++ {
		int limit
		//	Before doing anything else, take a copy of the i'th original sibling. The rest of this function will use data from the copies rather than the original pages since the original pages will be in the process of being overwritten.
		apCopy[i] = (MemoryPage*)(&aSpace1[pBt.pageSize + k * i])
		pOld = apCopy[i]
		memcpy(pOld, apOld[i], sizeof(MemoryPage))
		pOld.aData = raw.ByteSlice(pOld[1:])
		memcpy(pOld.aData, apOld[i].aData, pBt.pageSize)

		limit = pOld.nCell + pOld.nOverflow
		if pOld.nOverflow > 0 {
			for j = 0; j < limit; j++ {
				assert( nCell < nMaxCells )
				apCell[nCell] = pOld.FindOverflowCell(j)
				szCell[nCell] = pOld.cellSize(apCell[nCell])
				nCell++
			}
		} else {
			byte *aData = pOld.aData
			uint16 maskPage = pOld.maskPage
			uint16 cellOffset = pOld.cellOffset
			for j = 0; j < limit; j++ {
				assert( nCell < nMaxCells )
				apCell[nCell] = aData.FindCell(maskPage, cellOffset, j)
				szCell[nCell] = pOld.cellSize(apCell[nCell])
				nCell++
			}
		}
		if i < nOld - 1 && !IsLeafData {
			uint16 sz = (uint16)szNew[i]
			byte *pTemp
			assert( nCell < nMaxCells )
			szCell[nCell] = sz
			pTemp = &aSpace1[iSpace1]
			iSpace1 += sz
			assert( sz <= pBt.maxLocal + 23 )
			assert( iSpace1 <= (int)pBt.pageSize )
			memcpy(pTemp, apDiv[i], sz)
			apCell[nCell] = pTemp + IsLeafCorrection
			assert( IsLeafCorrection == 0 || IsLeafCorrection == 4 )
			szCell[nCell] = szCell[nCell] - IsLeafCorrection
			if !pOld.IsLeaf {
				assert( IsLeafCorrection == 0 )
				assert( pOld.hdrOffset == 0 )
				//	The right pointer of the child page pOld becomes the left pointer of the divider cell
				memcpy(apCell[nCell], &pOld.aData[8], 4)
			} else {
				assert( IsLeafCorrection == 4 )
				if szCell[nCell] < 4 {
					//	Do not allow any cells smaller than 4 bytes.
					szCell[nCell] = 4
				}
			}
			nCell++
		}
	}

	//	Figure out the number of pages needed to hold all nCell cells. Store this number in "k". Also compute szNew[] which is the total size of all cells on the i-th page and cntNew[] which is the index in apCell[] of the cell that divides page i from page i+1. cntNew[k] should equal nCell.
	//	Values computed by this block:
	//			k:				The total number of sibling pages
	//			szNew[i]:		Spaced used on the i-th sibling page.
	//			cntNew[i]:		Index in apCell[] and szCell[] for the first cell to the right of the i-th sibling page.
	//			usableSpace:	Number of bytes of space available on each sibling.
	usableSpace = pBt.usableSize - 12 + IsLeafCorrection
	for subtotal = k = i = 0; i < nCell; i++ {
		assert( i < nMaxCells )
		subtotal += szCell[i] + 2
		if subtotal > usableSpace {
			szNew[k] = subtotal - szCell[i]
			cntNew[k] = i
			if IsLeafData {
				i--
			}
			subtotal = 0
			k++
			if k > NB + 1 {
				rc = SQLITE_CORRUPT_BKPT
				goto balance_cleanup
			}
		}
	}
	szNew[k] = subtotal
	cntNew[k] = nCell
	k++

	//	The packing computed by the previous block is biased toward the siblings on the left side. The left siblings are always nearly full, while the right-most sibling might be nearly empty. This block of code attempts to adjust the packing of siblings to get a better balance.
	//	This adjustment is more than an optimization. The packing above might be so out of balance as to be illegal. For example, the right-most sibling might be completely empty. This adjustment is not optional.
	for i = k - 1; i > 0; i-- {
		int szRight = szNew[i];  /* Size of sibling on the right */
		int szLeft = szNew[i-1]; /* Size of sibling on the left */
		int r;              /* Index of right-most cell in left sibling */
		int d;              /* Index of first cell to the left of right sibling */

		r = cntNew[i - 1] - 1
		d = r + 1 - IsLeafData
		assert( d < nMaxCells )
		assert( r < nMaxCells )
		for szRight == 0 || szRight + szCell[d] + 2 <= szLeft - (szCell[r] + 2) {
			szRight += szCell[d] + 2
			szLeft -= szCell[r] + 2
			cntNew[i - 1]--
			r = cntNew[i - 1] - 1
			d = r + 1 - IsLeafData
		}
		szNew[i] = szRight
		szNew[i - 1] = szLeft
	}

	//	Either we found one or more cells (cntnew[0] > 0) or pPage is a virtual root page. A virtual root page is when the real root page is page 1 and we are the only child of that page.
	//	UPDATE: The assert() below is not necessarily true if the database file is corrupt. The corruption will be detected and reported later in this procedure so there is no need to act upon it now.

	//	Allocate k new pages.  Reuse old pages where possible.
	if apOld[0].pgno <= 1 {
		rc = SQLITE_CORRUPT_BKPT
		goto balance_cleanup
	}
	pageFlags = apOld[0].aData[0]
	for i = 0; i < k; i++ {
		MemoryPage *pNew;
		if i < nOld {
			pNew = apNew[i] = apOld[i]
			apOld[i] = 0
			rc = pNew.DbPage.Write()
			nNew++
			if rc != SQLITE_OK {
				goto balance_cleanup
			}
		} else {
			assert( i > 0 )
			if rc = allocateBtreePage(pBt, &pNew, &pgno, pgno, 0); rc != SQLITE_OK {
				goto balance_cleanup
			}
			apNew[i] = pNew
			nNew++

			//	Set the pointer-map entry for the new sibling page.
			if pBt.autoVacuum {
				if rc = pBt.Put(pNew.pgno, NON_ROOT_BTREE_PAGE, pParent.pgno); rc != SQLITE_OK {
					goto balance_cleanup
				}
			}
		}
	}

	//	Free any old pages that were not reused as new pages.
	for i < nOld {
		freePage(apOld[i], &rc)
		if rc != SQLITE_OK {
			goto balance_cleanup
		}
		apOld[i].Release()
		apOld[i] = nil
		i++
	}

	//	Put the new pages in accending order. This helps to keep entries in the disk file in order so that a scan of the table is a linear scan through the file. That in turn helps the operating system to deliver pages from the disk more rapidly.
	//	An O(n^2) insertion sort algorithm is used, but since n is never more than NB (a small constant), that should not be a problem.
	//	When NB == 3, this one optimization makes the database about 25% faster for large insertions and deletions.
	for i = 0; i < k - 1; i++ {
		int minV = apNew[i].pgno
		int minI = i
		for j = i + 1; j < k; j++ {
			if apNew[j].pgno < uint(minV) {
				minI = j
				minV = apNew[j].pgno
			}
		}
		if minI > i {
			MemoryPage *pT
			pT = apNew[i]
			apNew[i] = apNew[minI]
			apNew[minI] = pT
		}
	}

	Buffer(pRight).WriteUint32(apNew[nNew - 1].pgno)

	//	Evenly distribute the data in apCell[] across the new pages. Insert divider cells into pParent as necessary.
	j = 0
	for i = 0; i < nNew; i++ {
		//	Assemble the new sibling page.
		MemoryPage *pNew = apNew[i]
		assert( j < nMaxCells )
		zeroPage(pNew, pageFlags)
		assemblePage(pNew, cntNew[i] - j, &apCell[j], &szCell[j])
		assert( pNew.nCell > 0 || (nNew == 1 && cntNew[0] == 0) )
		assert( pNew.nOverflow == 0 )
		j = cntNew[i]

		//	If the sibling page assembled above was not the right-most sibling, insert a divider cell into the parent page.
		assert( i < nNew - 1 || j == nCell )
		if j < nCell {
			byte *pCell
			byte *pTemp
			int sz

			assert( j < nMaxCells )
			pCell = apCell[j]
			sz = szCell[j] + IsLeafCorrection
			pTemp = &aOvflSpace[iOvflSpace]
			switch {
			case !pNew.IsLeaf:
				memcpy(&pNew.aData[8], pCell, 4)
			case IsLeafData:
				//	If the tree is a IsLeaf-data tree, and the siblings are leaves, then there is no divider cell in apCell[]. Instead, the divider cell consists of the integer key for the right-most cell of the sibling-page assembled above only.
				j--
				info := ParsePtr(pNew, apCell[j])
				pCell = pTemp
				sz = 4 + len(pCell[4:]) - len(Buffer(pCell[4:]).WriteVarint64(info.nKey))
				pTemp = nil
			default:
				pCell -= 4
				//	Obscure case for non-IsLeaf-data trees: If the cell at pCell was previously stored on a IsLeaf node, and its reported size was 4 bytes, then it may actually be smaller than this (see CellInfo::ParsePtr(), 4 bytes is the minimum size of any cell). But it is important to pass the correct size to InsertCell(), so reparse the cell now.
				//	Note that this can never happen in an SQLite data file, as all cells are at least 4 bytes. It only happens in b-trees used to evaluate "IN (SELECT ...)" and similar clauses.
				if szCell[j] == 4 {
					assert( IsLeafCorrection == 4 )
					sz = pParent.cellSize(pCell)
				}
			}
			iOvflSpace += sz
			assert( sz <= pBt.maxLocal + 23 )
			assert( iOvflSpace <= int(pBt.pageSize) )
			if rc = pParent.InsertCell(nxDiv, pCell[:sz], pTemp, pNew.pgno); rc != SQLITE_OK {
				goto balance_cleanup
			}
			j++
			nxDiv++
		}
	}
	assert( j == nCell )
	assert( nOld > 0 )
	assert( nNew > 0 )
	if (pageFlags & PTF_IsLeaf) == 0 {
		byte *zChild = &apCopy[nOld-1].aData[8]
		memcpy(&apNew[nNew-1].aData[8], zChild, 4)
	}

	if isRoot && pParent.nCell == 0 && pParent.hdrOffset <= apNew[0].nFree {
		//	The root page of the b-tree now contains no cells. The only sibling page is the right-child of the parent. Copy the contents of the child page into the parent, decreasing the overall height of the b-tree structure by one. This is described as the "balance-shallower" sub-algorithm in some documentation.
		//	If this is an auto-vacuum database, the call to copyNodeContent() sets all pointer-map entries corresponding to database image pages for which the pointer is stored within the content being copied.
		//	The second assert below verifies that the child page is defragmented (it must be, as it was just reconstructed using assemblePage()). This is important if the parent page happens to be page 1 of the database image.
		assert( nNew == 1 )
		assert( apNew[0].nFree == (Buffer(apNew[0].aData[5]).ReadUint16()) - apNew[0].cellOffset - apNew[0].nCell * 2) )
		copyNodeContent(apNew[0], pParent, &rc)
		freePage(apNew[0], &rc)
	} else if pBt.autoVacuum {
		//	Fix the pointer-map entries for all the cells that were shifted around. There are several different types of pointer-map entries that need to be dealt with by this routine. Some of these have been set already, but many have not. The following is a summary:
		//			1) The entries associated with new sibling pages that were not siblings when this function was called. These have already been set. We don't need to worry about old siblings that were moved to the free-list - the freePage() code has taken care of those.
		//			2) The pointer-map entries associated with the first overflow page in any overflow chains used by new divider cells. These have also already been taken care of by the InsertCell() code.
		//			3) If the sibling pages are not leaves, then the child pages of cells stored on the sibling pages may need to be updated.
		//			4) If the sibling pages are not internal IsIntegerKey nodes, then any overflow pages used by these cells may need to be updated (internal IsIntegerKey nodes never contain pointers to overflow pages).
		//			5) If the sibling pages are not leaves, then the pointer-map entries for the right-child pages of each sibling may need to be updated.
		//	Cases 1 and 2 are dealt with above by other code. The next block deals with cases 3 and 4 and the one after that, case 5. Since setting a pointer map entry is a relatively expensive operation, this code only sets pointer map entries for child or overflow pages that have actually moved between pages.
		MemoryPage *pNew = apNew[0]
		MemoryPage *pOld = apCopy[0]
		int nOverflow = pOld.nOverflow
		int iNextOld = pOld.nCell + nOverflow
		int iOverflow = (nOverflow ? pOld.aiOvfl[0] : -1)
		j = 0;                             /* Current 'old' sibling page */
		k = 0;                             /* Current 'new' sibling page */
		for i = 0; i < nCell; i++ {
			int isDivider = 0
			for i == iNextOld {
				//	Cell i is the cell immediately following the last cell on old sibling page j. If the siblings are not IsLeaf pages of an IsIntegerKey b-tree, then cell i was a divider cell.
				assert( j + 1 < ArraySize(apCopy) )
				j++
				pOld = apCopy[j]
				iNextOld = i + !IsLeafData + pOld.nCell + pOld.nOverflow
				if pOld.nOverflow {
					nOverflow = pOld.nOverflow
					iOverflow = i + !IsLeafData + pOld.aiOvfl[0]
				}
				isDivider = !IsLeafData
			}

			assert( nOverflow > 0 || iOverflow < i )
			assert( nOverflow < 2 || pOld.aiOvfl[0] == pOld.aiOvfl[1] - 1)
			assert(nOverflow < 3 || pOld.aiOvfl[1] == pOld.aiOvfl[2] - 1)
			if i == iOverflow {
				isDivider = 1
				if nOverflow--; nOverflow > 0 {
					iOverflow++
				}
			}

			if i == cntNew[k] {
				//	Cell i is the cell immediately following the last cell on new sibling page k. If the siblings are not IsLeaf pages of an IsIntegerKey b-tree, then cell i is a divider cell.
				k++
				pNew = apNew[k]
				if !IsLeafData {
					continue
				}
			}
			assert( j < nOld )
			assert( k < nNew )

			//	If the cell was originally divider cell (and is not now) or an overflow cell, or if the cell was located on a different sibling page before the balancing, then the pointer map entries associated with any child or overflow pages need to be updated.
			if isDivider || pOld.pgno != pNew.pgno {
				if !IsLeafCorrection {
					rc = pBt.Put(Buffer(apCell[i:]).ReadUint32(), NON_ROOT_BTREE_PAGE, pNew.pgno)
				}
				if szCell[i] > pNew.minLocal {
					rc = pNew.PutOvflPtr(apCell[i])
				}
			}
		}

		if !IsLeafCorrection {
			for i = 0; i < nNew; i++ {
				uint32 key = Buffer(apNew[i].aData[8:]).ReadUint32()
				rc = pBt.Put(key, NON_ROOT_BTREE_PAGE, apNew[i].pgno)
			}
		}
	}

	assert( pParent.isInit )

	//	Cleanup before returning.
balance_cleanup:
	sqlite3ScratchFree(apCell)
	for i = 0; i < nOld; i++ {
		apOld[i].Release()
	}
	for i = 0; i < nNew; i++ {
		apNew[i].Release()
	}
	return rc
}

//	Insert a new cell on pPage at cell index "i". pCell points to the content of the cell.
//	If the cell content will fit on the page, then put it there. If it will not fit, then make a copy of the cell content into pTemp if pTemp is not null. Regardless of pTemp, allocate a new entry in pPage.apOvfl[] and make it point to the cell content (either in pTemp or the original pCell) and also record its index. Allocating a new entry in pPage.CellIndices[] implies that pPage.nOverflow is incremented.
//	If nSkip is non-zero, then do not copy the first nSkip bytes of the cell. The caller will overwrite them after this function returns. If nSkip is non-zero, then pCell may not point to an invalid memory location (but pCell+nSkip is always valid).
func (pPage *MemoryPage) InsertCell(i int, cell, temp_space Buffer, child PageNumber) (rc int) {
	size := len(cell)
	assert( i >= 0 && i <= pPage.nCell + pPage.nOverflow )
	assert( pPage.nCell <= MX_CELL(pPage.BtShared) && MX_CELL(pPage.BtShared) <= 10921 )
	assert( pPage.nOverflow <= len(pPage.apOvfl) )
	assert( len(pPage.apOvfl) == len(pPage.aiOvfl) )
	//	The cell should normally be sized correctly. However, when moving a malformed cell from a IsLeaf page to an interior page, if the cell size wanted to be less than 4 but got rounded up to 4 on the IsLeaf, then size might be less than 8 (IsLeaf-size + pointer) on the interior node. Hence the term after the || in the following assert().
	assert( size == pPage.cellSize(pCell) || (size == 8 && child > 0) )

	nSkip := 0
	if child != 0 {
		nSkip = 4
	}

	if pPage.nOverflow || size + 2 > pPage.nFree {
		if temp_space != nil {
			copy(temp_space[nSkip:], cell[nSkip:size - nSkip])
			cell = temp_space
		}
		if child != 0 {
			Buffer(cell).WriteUint32(child)
		}
		pPage.nOverflow++
		j := pPage.nOverflow
		assert( j < len(pPage.apOvfl) )
		pPage.apOvfl[j] = temp_space
		pPage.aiOvfl[j] = uint16(i)
	} else {
		if rc = pPage.DbPage.Write(); rc == SQLITE_OK {
			data := Buffer(pPage.aData)
			cellOffset = pPage.cellOffset				//	Address of first cell pointer in data[]
			ins := cellOffset + 2 * i					//	Index in data[] where new cell pointer is inserted
			if idx, rc = pPage.AllocateSpace(size); rc == SQLITE_OK {
				pPage.nCell++
				pPage.nFree -= uint16(2 + size)
				copy(data[idx + nSkip:], cell[nSkip:size - nSkip])
				if child != 0 {
					Buffer(data[idx:]).WriteUint32(child)
				}
				copy(data[ins + 2:], data[ins:])		//	shift the contents to create a write-window for insertion
				data[ins:].WriteUint16(idx)
				data[pPage.hdrOffset + 3:].WriteUint16(pPage.nCell)
				if pPage.BtShared.autoVacuum {
					//	The cell may contain a pointer to an overflow page. If so, write the entry for the overflow page into the pointer map.
					pRC = pPage.PutOvflPtr(cell)
				}
			}
		}
	}
}

//	Allocate nByte bytes of space from within the B-Tree page passed as the first argument. Write into *pIdx the index into pPage.aData[] of the first byte of allocated space. Return either SQLITE_OK or an error code (usually SQLITE_CORRUPT).
//	The caller guarantees that there is sufficient space to make the allocation. This routine might need to defragment in order to bring all the space together, however. This routine will avoid using the first two bytes past the cell pointer area since presumably this allocation is being made in order to insert a new cell, so we will also end up needing a new cell pointer.
func (pPage *MemoryPage) AllocateSpace(nByte int) (index, rc int) {
	hdr := pPage.hdrOffset					//	Local cache of pPage.hdrOffset
	data := Buffer(pPage.aData)

	assert( pPage.BtShared )
	assert( nByte >= 0 )					//	Minimum cell size is 4
	assert( pPage.nFree >= nByte )
	assert( pPage.nOverflow == 0 )
	usableSize := pPage.BtShared.usableSize
	assert( nByte < usableSize - 8 )

	fragmented_bytes := data[hdr + 7]
	if pPage.IsLeaf {
		assert( pPage.cellOffset == hdr + 8 )
	} else {
		assert( pPage.cellOffset == hdr + 12 )
	}
	gap := pPage.cellOffset + 2 * pPage.nCell						//	First byte of gap between cell pointers and cell content
	if index = data[hdr + 5].ReadCompressedIntNotZero(); gap > index {
		return SQLITE_CORRUPT_BKPT
	}
	if fragmented_bytes >= 60 {
		//	Always defragment highly fragmented pages
		if rc = pPage.Defragment(); rc != SQLITE_OK {
			return
		}
		index = data[hdr + 5].ReadCompressedIntNotZero()
	} else if gap + 2 <= index {
		//	Search the freelist looking for a free slot big enough to satisfy the request. The allocation is made from the first free slot in the list that is large enough to accomadate it.
		addr := hdr + 1
		for pc = data[addr].ReadUint16(); pc > 0; pc = data[addr].ReadUint16() {
			if pc > usableSize - 4 || pc < addr + 4 {
				return SQLITE_CORRUPT_BKPT
			}
			if slot_size := data[pc + 2].ReadUint16(); slot_size >= nByte {
				x := size - nByte
				switch {
				case x < 4:
					//	Remove the slot from the free-list. Update the number of fragmented bytes within the page.
					copy(&data[addr:], &data[pc:], 2)
					data[hdr + 7] = byte(fragmented_bytes + x)
				case slot_size + pc > usableSize:
					return SQLITE_CORRUPT_BKPT
				default:
					//	The slot remains on the free-list. Reduce its size to account for the portion used by the new allocation.
					data[pc + 2:].WriteUint16(x)
				}
				index = pc + x
				return
			}
			addr = pc
		}
	}

	//	Check to make sure there is enough space in the gap to satisfy the allocation. If not, defragment.
	if gap + 2 + nByte > index {
		if rc = pPage.Defragment(); rc != SQLITE_OK {
			return rc
		}
		index = data[hdr + 5].ReadCompressedIntNotZero()
		assert( gap + nByte <= index )
	}


	// Allocate memory from the gap in between the cell pointer array and the cell content area. The Initialize() call has already validated the freelist. Given that the freelist is valid, there is no way that the allocation can extend off the end of the page. The assert() below verifies the previous sentence.
	index -= nByte
	data[hdr + 5:].WriteUint16(top)
	assert( index + nByte <= int(pPage.BtShared.usableSize) )
	return
}

//	Compute the total number of bytes that a Cell needs in the cell data area of the btree-page. The return number includes the cell
//	data header and the local payload, but not any overflow page or the space used by the cell pointer.
func (pPage *MemoryPage) cellSize(pCell Buffer) uint16 {
	var nSize	uint32

	buffer := Buffer(pCell[pPage.childPtrSize:])
	if pPage.IsIntegerKey {
		if pPage.HasData {
			nSize, buffer = buffer.ReadVarint32()
		} else {
			nSize = 0
		}

		//	pIter now points at the 64-bit integer key value, a variable length integer. The following block moves pIter to point at the first byte past the end of the key value.
		for i, v := range buffer {
			if !(v & 0x80 != 0 && i < 9) {
				break
			}
		}

	} else {
		nSize, buffer = buffer.ReadVarint32()
	}

	if nSize > pPage.maxLocal {
		minLocal := pPage.minLocal
		if nSize = minLocal + (nSize - minLocal) % (pPage.BtShared.usableSize - 4); nSize > pPage.maxLocal {
			nSize = minLocal
		}
		nSize += 4
	}
	if nSize += len(pCell) - len(buffer); nSize < 4 {
		//	The minimum size of any cell is 4 bytes.
		nSize = 4
	}
	assert( nSize == debuginfo.nSize )
	return uint16(nSize)
}

//	If the cell pCell, part of page pPage contains a pointer to an overflow page, insert an entry into the pointer-map for the overflow page.
func (p *MemoryPage) PutOvflPtr(cell []byte) (rc int) {
	assert( cell != nil )
	info := ParsePtr(p, cell)
	if p.IsIntegerKey {
		assert( info.nData == info.nPayload )
	} else {
		assert( info.nData + info.nKey == info.nPayload )
	}
	if info.iOverflow != 0 {
		ovfl = PageNumber(Buffer(cell[info.iOverflow:]).ReadUint32())
		rc = p.BtShared.Put(ovfl, FIRST_OVERFLOW_PAGE, p.pgno)
	}
	return
}
