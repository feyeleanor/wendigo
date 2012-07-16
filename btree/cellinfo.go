package btree

//	An instance of the following structure is used to hold information about a cell.
//	The parseCellPtr() function fills in this structure based on information extract from the raw disk page.
type CellInfo struct {
	Key			int64		//	The key for INTKEY tables, or number of bytes in key
	Cell		[]byte		//	Pointer to the start of cell content
	Data		uint32		//	Number of bytes of data
	Payload		uint32		//	Total amount of payload
	Header		uint32		//	Size of the cell content header in bytes
	Local		uint32		//	Amount of payload held locally
	Overflow	uint16		//	Offset to overflow page number.  Zero if no overflow
	Size		uint16		//	Size of the cell content on the main b-tree page
}

//	Parse a cell content block and fill in the CellInfo structure.

func (p *CellInfo) Parse(page *MemoryPage, cell int) {
	p.ParsePtr(page, page.FindCell(cell))
}

func (p *CellInfo) ParsePtr(page *MemoryPage, cell Buffer) {
	var payload		uint32			//	Number of bytes of cell payload
	var buf			Buffer

	p.Cell = cell
	assert(page.leaf == 0 || page.leaf == 1)
	n := page.childPtrSize
	assert(n == 4 - 4 * page.leaf)
	if page.intKey {
		if page.hasData {
			payload, buf = Buffer(cell[n:]).GetVarint32()
			n += len(buf) - len(cell[n:])
		} else {
			payload = 0
		}
		p.Key, buf = Buffer(p[n:]).GetVarint64()
		n += len(buf) - len(p[n:])
		p.Data = payload
	} else {
		p.Data = 0
		payload, buf = Buffer(cell[n:]).GetVarint32()
		n += len(buf) - len(cell[n:])
		p.Key = payload
	}
	p.Payload = payload
	p.Header = n
	if payload <= page.maxLocal {
		//	This is the (easy) common case where the entire payload fits on the local page. No overflow is required.
		if p.Size = uint16(n + payload); p.Size < 4 {
			p.Size = 4
		}
		p.Local = uint16(payload)
		p.Overflow = 0
	} else {
		//	If the payload will not fit completely on the local page, we have to decide how much to store locally and how much to spill onto
		//	overflow pages. The strategy is to minimize the amount of unused space on overflow pages while keeping the amount of local storage
		//	in between minLocal and maxLocal.
		//
		//	Warning:  changing the way overflow payload is distributed in any way will result in an incompatible file format.
		if surplus_space := minimum_local_payload + (payload - minimum_local_payload) % (page.pBt.usableSize - 4); surplus_payload <= maximum_local_payload {
			p.Local = uint16(surplus_space)
		} else {
			p.Local = uint16(minimum_local_payload)
		}
		p.Overflow = uint16(p.Local + n)
		p.Size = p.Overflow + 4
	}
}