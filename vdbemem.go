/* This file contains code use to manipulate "Mem" structure.  A "Mem"
** stores a single value in the VDBE.  Mem is an opaque structure visible
** only within the VDBE.  Interface routines refer to a Mem using the
** name sqlite_value
*/

/*
** If pMem is an object with a valid string representation, this routine
** ensures the internal encoding for the string representation is
** 'desiredEnc' is SQLITE_UTF8.
**
** If pMem is not a string object, or the encoding of the string
** representation is already stored using the requested encoding, then this
** routine is a no-op.
**
** SQLITE_OK is returned if the conversion is successful (or not required).
** SQLITE_NOMEM may be returned if a malloc() fails during conversion
** between formats.
*/
int sqlite3VdbeChangeEncoding(Mem *pMem, int desiredEnc){
  int rc;
  assert( (pMem.flags&MEM_RowSet)==0 );
  assert( desiredEnc==SQLITE_UTF8 )
  if( !(pMem.flags&MEM_Str) || pMem.enc==desiredEnc ){
    return SQLITE_OK;
  }
  return SQLITE_ERROR;
}

/*
** Make sure pMem.z points to a writable allocation of at least 
** n bytes.
**
** If the third argument passed to this function is true, then memory
** cell pMem must contain a string or blob. In this case the content is
** preserved. Otherwise, if the third parameter to this function is false,
** any current string or blob value may be discarded.
**
** This function sets the MEM_Dyn flag and clears any xDel callback.
** It also clears MEM_Ephem and MEM_Static. If the preserve flag is 
** not set, Mem.n is zeroed.
*/
 int sqlite3VdbeMemGrow(Mem *pMem, int n, int preserve){
  assert( 1 >=
    ((pMem.zMalloc && pMem.zMalloc==pMem.z) ? 1 : 0) +
    (((pMem.flags&MEM_Dyn)&&pMem.xDel) ? 1 : 0) + 
    ((pMem.flags&MEM_Ephem) ? 1 : 0) + 
    ((pMem.flags&MEM_Static) ? 1 : 0)
  );
  assert( (pMem.flags&MEM_RowSet)==0 );

  /* If the preserve flag is set to true, then the memory cell must already
  ** contain a valid string or blob value.  */
  assert( preserve==0 || pMem.flags&(MEM_Blob|MEM_Str) );

  if( n<32 ) n = 32;
  if( sqlite3DbMallocSize(pMem.db, pMem.zMalloc)<n ){
    if( preserve && pMem.z==pMem.zMalloc ){
      pMem.z = pMem.zMalloc = sqlite3DbReallocOrFree(pMem.db, pMem.z, n);
      preserve = 0;
    }else{
      pMem.zMalloc = sqlite3DbMallocRaw(pMem.db, n);
    }
  }

  if( pMem.z && preserve && pMem.zMalloc && pMem.z!=pMem.zMalloc ){
    memcpy(pMem.zMalloc, pMem.z, pMem.n);
  }
  if( pMem.flags&MEM_Dyn && pMem.xDel ){
    assert( pMem.xDel!=SQLITE_DYNAMIC );
    pMem.xDel((void *)(pMem.z));
  }

  pMem.z = pMem.zMalloc;
  if( pMem.z==0 ){
    pMem.flags = MEM_Null;
  }else{
    pMem.flags &= ~(MEM_Ephem|MEM_Static);
  }
  pMem.xDel = 0;
  return (pMem.z ? SQLITE_OK : SQLITE_NOMEM);
}

/*
** Make the given Mem object MEM_Dyn.  In other words, make it so
** that any TEXT or BLOB content is stored in memory obtained from
** malloc().  In this way, we know that the memory is safe to be
** overwritten or altered.
**
** Return SQLITE_OK on success or SQLITE_NOMEM if malloc fails.
*/
 int sqlite3VdbeMemMakeWriteable(Mem *pMem){
  int f;
  assert( (pMem.flags&MEM_RowSet)==0 );
  ExpandBlob(pMem)
  f = pMem.flags;
  if( (f&(MEM_Str|MEM_Blob)) && pMem.z!=pMem.zMalloc ){
    if( sqlite3VdbeMemGrow(pMem, pMem.n + 2, 1) ){
      return SQLITE_NOMEM;
    }
    pMem.z[pMem.n] = 0;
    pMem.z[pMem.n+1] = 0;
    pMem.flags |= MEM_Term;
  }

  return SQLITE_OK;
}


/*
** Make sure the given Mem is \u0000 terminated.
*/
 int sqlite3VdbeMemNulTerminate(Mem *pMem){
  if( (pMem.flags & MEM_Term)!=0 || (pMem.flags & MEM_Str)==0 ){
    return SQLITE_OK;   /* Nothing to do */
  }
  if( sqlite3VdbeMemGrow(pMem, pMem.n+2, 1) ){
    return SQLITE_NOMEM;
  }
  pMem.z[pMem.n] = 0;
  pMem.z[pMem.n+1] = 0;
  pMem.flags |= MEM_Term;
  return SQLITE_OK;
}

//	Add MEM_Str to the set of representations for the given Mem. Converting a BLOB to a string is a no-op.
//	Existing representations MEM_Int and MEM_Real are *not* invalidated.
//	A MEM_Null value will never be passed to this function. This function is used for converting values to text for returning to the user (i.e. via Mem_text()), or for ensuring that values to be used as btree keys are strings. In the former case a NULL pointer is returned the user and the later is an internal programming error.
func (pMem *Mem) Stringify(enc int) (rc int) {
	fg := pMem.flags
	nByte := 32

	assert( fg & (MEM_Str | MEM_Blob) == 0 )
	assert( fg & (MEM_Int | MEM_Real) != 0 )
	assert( pMem.flags & MEM_RowSet == 0 )
	assert( EIGHT_BYTE_ALIGNMENT(pMem) )

	if sqlite3VdbeMemGrow(pMem, nByte, 0) {
		return SQLITE_NOMEM
	}

	if fg & MEM_Int != 0 {
		pMem.z = fmt.Sprintf("%v", pMem.Integer())
	} else {
		assert( fg & MEM_Real )
		pMem.z = fmt.Sprintf("%!.15g", pMem.r)
	}
	pMem.enc = SQLITE_UTF8
	pMem.flags |= MEM_Str | MEM_Term
	sqlite3VdbeChangeEncoding(pMem, enc)
	return
}

//	Memory cell pMem contains the context of an aggregate function. This routine calls the finalize method for that function. The result of the aggregate is stored back into pMem.
//	Return SQLITE_ERROR if the finalizer reports an error. SQLITE_OK otherwise.
func (pMem *Mem) Finalize(pFunc *FuncDef) (rc int) {
	if pFunc != nil && pFunc.xFinalize != nil {
		assert( (pMem.flags & MEM_Null) != 0 || pFunc == pMem.u.pDef )
		ctx := &sqlite3_context{
			s.flags:	MEM_Null,
			s.db:		pMem.db,
			pMem:		pMem,
			pFunc:		pFunc,
		}
		pFunc.xFinalize(ctx)				//	IMP: R-24505-23230
		assert( pMem.flags & MEM_Dyn == 0 && pMem.xDel == nil )
		pMem.zMalloc = nil
		copy(pMem, ctx.s)
		rc = ctx.isError
	}
	return
}

//	If the memory cell contains a string value that must be freed by invoking an external callback, free it now. Calling this function does not free any Mem.zMalloc buffer.
func (p *Mem) ReleaseExternal() {
	switch {
	case p.flags & MEM_Agg != 0:
		p.Finalize(p.u.pDef)
		assert( (p.flags & MEM_Agg) == 0 )
		p.Release()
	case p.flags & MEM_Dyn && p.xDel:
		assert( p.flags & MEM_RowSet == 0 )
		assert( p.xDel != SQLITE_DYNAMIC )
		p.xDel(p.z)
		p.xDel = nil
	case p.flags & MEM_RowSet != 0:
		p.u.pRowSet.Clear()
	case p.flags & MEM_Frame != 0:
		p.SetNull()
	}
}

//	Release any memory held by the Mem. This may leave the Mem in an inconsistent state, for example with (Mem.z == "") and (Mem.Type == SQLITE_TEXT).
func (p *Mem) Release() {
	VdbeMemRelease(p);
	p.zMalloc = nil
	p.z = ""
	p.xDel = nil
}

/*
** Convert a 64-bit IEEE double into a 64-bit signed integer.
** If the double is too large, return 0x8000000000000000.
**
** Most systems appear to do this simply by assigning
** variables and without the extra range tests.  But
** there are reports that windows throws an expection
** if the floating point value is out of range. (See ticket #2880.)
** Because we do not completely understand the problem, we will
** take the conservative approach and always do range tests
** before attempting the conversion.
*/
static int64 doubleToInt64(double r){
  /*
  ** Many compilers we encounter do not define constants for the
  ** minimum and maximum 64-bit integers, or they define them
  ** inconsistently.  And many do not understand the "LL" notation.
  ** So we define our own static constants here using nothing
  ** larger than a 32-bit integer constant.
  */
  static const int64 maxInt = LARGEST_INT64;
  static const int64 minInt = SMALLEST_INT64;

  if( r<(double)minInt ){
    return minInt;
  }else if( r>(double)maxInt ){
    /* minInt is correct here - not maxInt.  It turns out that assigning
    ** a very large positive number to an integer results in a very large
    ** negative integer.  This makes no sense, but it is what x86 hardware
    ** does so for compatibility we will do the same in software. */
    return minInt;
  }else{
    return (int64)r;
  }
}

/*
** Return some kind of integer value which is the best we can do
** at representing the value that *pMem describes as an integer.
** If pMem is an integer, then the value is exact.  If pMem is
** a floating-point then the value returned is the integer part.
** If pMem is a string or blob, then we make an attempt to convert
** it into a integer and return that.  If pMem represents an
** an SQL-NULL value, return 0.
**
** If pMem represents a string value, its encoding might be changed.
*/
 int64 sqlite3VdbeIntValue(Mem *pMem){
  int flags;
  assert( EIGHT_BYTE_ALIGNMENT(pMem) );
  flags = pMem.flags;
  if( flags & MEM_Int ){
    return pMem.Value
  }else if( flags & MEM_Real ){
    return doubleToInt64(pMem.r);
  }else if( flags & (MEM_Str|MEM_Blob) ){
    int64 value = 0;
    assert( pMem.z || pMem.n==0 );
    Atoint64(pMem.z, &value, pMem.n, pMem.enc);
    return value;
  }else{
    return 0;
  }
}

/*
** Return the best representation of pMem that we can get into a
** double.  If pMem is already a double or an integer, return its
** value.  If it is a string or blob, try to convert it to a double.
** If it is a NULL, return 0.0.
*/
 double sqlite3VdbeRealValue(Mem *pMem){
  assert( EIGHT_BYTE_ALIGNMENT(pMem) );
  if( pMem.flags & MEM_Real ){
    return pMem.r;
  }else if( pMem.flags & MEM_Int ){
    return float64(pMem.Integer())
  }else if( pMem.flags & (MEM_Str|MEM_Blob) ){
    double val = (double)0;
    sqlite3AtoF(pMem.z, &val, pMem.n, pMem.enc);
    return val;
  }else{
    return (double)0;
  }
}

//	The MEM structure is already a MEM_Real. Try to also make it a MEM_Int if we can.
func (pMem *Mem) IntegerAffinity() {
	assert( pMem.flags & MEM_Real != 0 )
	assert( (pMem.flags & MEM_RowSet) == 0 )
	assert( EIGHT_BYTE_ALIGNMENT(pMem) )

	pMem.Store(doubleToInt64(pMem.r))

	//	Only mark the value as an integer if
	//		(1) the round-trip conversion real.int.real is a no-op, and
	//		(2) The integer is neither the largest nor the smallest possible integer (ticket #3922)
	//	The second and third terms in the following conditional enforces the second condition under the assumption that addition overflow causes values to wrap around. On x86 hardware, the third term is always true and could be omitted. But we leave it in because other architectures might behave differently.
	if pMem.r == float64(pMem.Value) && pMem.Value > SMALLEST_INT64 && pMem.Value < LARGEST_INT64 {
		pMem.flags |= MEM_Int
	}
}

//	Convert pMem to type integer. Invalidate any prior representations.
func (pMem *Mem) Integerify() (rc int) {
	assert( pMem.flags & MEM_RowSet == 0 )
	assert( EIGHT_BYTE_ALIGNMENT(pMem) )

	pMem.Store(sqlite3VdbeIntValue(pMem))
	pMem.SetTypeFlag(MEM_Int)
	return
}

//	Convert pMem so that it is of type MEM_Real. Invalidate any prior representations.
func (pMem *Mem) Realify() (rc int) {
	assert( EIGHT_BYTE_ALIGNMENT(pMem) )
	pMem.r = sqlite3VdbeRealValue(pMem)
	pMem.SetTypeFlag(MEM_Real)
	return
}

//	Convert pMem so that it has types MEM_Real or MEM_Int or both. Invalidate any prior representations.
//	Every effort is made to force the conversion, even if the input is a string that does not look completely like a number. Convert as much of the string as we can and ignore the rest.
func (pMem *Mem) Numerify() (rc int) {
	if pMem.flags & (MEM_Int | MEM_Real | MEM_Null) == 0 {
		assert( pMem.flags & (MEM_Blob | MEM_Str) != 0 )
		if Atoint64(pMem.z, &pMem.Value, pMem.n, pMem.enc) == 0 {
			pMem.SetTypeFlag(MEM_Int)
		} else {
			pMem.r = sqlite3VdbeRealValue(pMem)
			pMem.SetTypeFlag(MEM_Real)
			pMem.IntegerAffinity()
		}
	}
	assert( pMem.flags & (MEM_Real | MEM_Null) != 0 )
	pMem.flags &= ~(MEM_Str | MEM_Blob)
	return
}

//	Delete any previous value and set the value stored in *pMem to NULL.
func (pMem *Mem) SetNull() {
	if pMem.flags & MEM_Frame != 0 {
		pFrame := pMem.u.pFrame
		pFrame.pParent = pFrame.v.pDelFrame
		pFrame.v.pDelFrame = pFrame
	}
	if pMem.flags & MEM_RowSet != 0 {
		pMem.u.pRowSet.Clear()
	}
	pMem.Type = SQLITE_NULL
}

//	Delete any previous value and set the value to be a BLOB of length n containing all zeros.
func (pMem *Mem) SetZeroBlob(n Zeroes) {
	pMem.Release()
	pMem.flags = MEM_Blob
	pMem.Type = SQLITE_BLOB
	pMem.n = 0
	if n < 0 {
		n = 0
	}
	pMem.Value = n
	pMem.enc = SQLITE_UTF8
}

//	Delete any previous value and set the value stored in *pMem to val, manifest type INTEGER.
func (pMem *Mem) SetInt64(val int64) {
	pMem.Release()
	pMem.Store(val)
	pMem.Type = SQLITE_INTEGER
}

/*
** Delete any previous value and set the value stored in *pMem to val,
** manifest type REAL.
*/
 void sqlite3VdbeMemSetDouble(Mem *pMem, double val){
  if math.IsNaN(val) {
    pMem.SetNull()
  }else{
    pMem.Release()
    pMem.r = val;
    pMem.flags = MEM_Real;
    pMem.Type = SQLITE_FLOAT;
  }
}

/*
** Delete any previous value and set the value of pMem to be an
** empty boolean index.
*/
 void sqlite3VdbeMemSetRowSet(Mem *pMem){
  sqlite3 *db = pMem.db;
  assert( db!=0 );
  assert( (pMem.flags & MEM_RowSet)==0 );
  pMem.Release()
  pMem.zMalloc = sqlite3DbMallocRaw(db, 64);
  if( db.mallocFailed ){
    pMem.flags = MEM_Null;
  }else{
    assert( pMem.zMalloc );
    pMem.u.pRowSet = sqlite3RowSetInit(db, pMem.zMalloc, 
                                       sqlite3DbMallocSize(db, pMem.zMalloc));
    assert( pMem.u.pRowSet!=0 );
    pMem.flags = MEM_RowSet;
  }
}

//	Return true if the Mem object contains a TEXT or BLOB that is too large - whose size exceeds SQLITE_MAX_LENGTH.
func (p *Mem) VdbeMemTooBig() (ok bool) {
	assert( p.db != nil )
	if p.flags & (MEM_Str | MEM_Blob) {
		ok = p.ByteLen() < 0
	}
	return
}


/*
** Size of struct Mem not including the Mem.zMalloc member.
*/
#define MEMCELLSIZE (size_t)(&(((Mem *)0).zMalloc))

/*
** Make an shallow copy of pFrom into pTo.  Prior contents of
** pTo are freed.  The pFrom.z field is not duplicated.  If
** pFrom.z is used, then pTo.z points to the same thing as pFrom.z
** and flags gets srcType (either MEM_Ephem or MEM_Static).
*/
 void sqlite3VdbeMemShallowCopy(Mem *pTo, const Mem *pFrom, int srcType){
  assert( (pFrom.flags & MEM_RowSet)==0 );
  VdbeMemRelease(pTo);
  memcpy(pTo, pFrom, MEMCELLSIZE);
  pTo.xDel = 0;
  if( (pFrom.flags&MEM_Static)==0 ){
    pTo.flags &= ~(MEM_Dyn|MEM_Static|MEM_Ephem);
    assert( srcType==MEM_Ephem || srcType==MEM_Static );
    pTo.flags |= srcType;
  }
}

/*
** Make a full copy of pFrom into pTo.  Prior contents of pTo are
** freed before the copy is made.
*/
 int sqlite3VdbeMemCopy(Mem *pTo, const Mem *pFrom){
  int rc = SQLITE_OK;

  assert( (pFrom.flags & MEM_RowSet)==0 );
  VdbeMemRelease(pTo);
  memcpy(pTo, pFrom, MEMCELLSIZE);
  pTo.flags &= ~MEM_Dyn;

  if( pTo.flags&(MEM_Str|MEM_Blob) ){
    if( 0==(pFrom.flags&MEM_Static) ){
      pTo.flags |= MEM_Ephem;
      rc = sqlite3VdbeMemMakeWriteable(pTo);
    }
  }

  return rc;
}

/*
** Transfer the contents of pFrom to pTo. Any existing value in pTo is
** freed. If pFrom contains ephemeral data, a copy is made.
**
** pFrom contains an SQL NULL when this routine returns.
*/
 void sqlite3VdbeMemMove(Mem *pTo, Mem *pFrom){
  assert( pFrom.db==0 || pTo.db==0 || pFrom.db==pTo.db );

  pTo.Release()
  memcpy(pTo, pFrom, sizeof(Mem));
  pFrom.flags = MEM_Null;
  pFrom.xDel = 0;
  pFrom.zMalloc = 0;
}

//	Change the value of a Mem to be a string or a BLOB.
//	The memory management strategy depends on the value of the xDel parameter. If the value passed is SQLITE_TRANSIENT, then the string is copied into a (possibly existing) buffer managed by the Mem structure. Otherwise, any existing buffer is freed and the pointer copied.
//	If the string is too large (if it exceeds the SQLITE_LIMIT_LENGTH size limit) then no memory allocation occurs. If the string can be stored without allocating memory, then it is. If a memory allocation is required to store the string, then value of pMem is unchanged. In either case, SQLITE_TOOBIG is returned.
func (pMem *Mem) SetStr(z *string, enc byte, xDel func(interface{}) interface{}) (rc int) {
	assert( (pMem.flags & MEM_RowSet) == 0 )

	//	If z is a NULL pointer, set pMem to contain an SQL NULL.
	if z == nil {
		pMem.SetNull()
	} else {
		var flags	uint16
		if enc == 0 {
			flags = MEM_Blob
		} else {
			flags = MEM_Str
		}

		//	The following block sets the new values of Mem.z and Mem.xDel. It also sets a flag in local variable "flags" to indicate the memory management (one of MEM_Dyn or MEM_Static).
		switch xDel {
		case SQLITE_TRANSIENT:
			pMem.z = append("", *z)
		case SQLITE_DYNAMIC:
			pMem.Release()
			pMem.z = *z
			pMem.zMalloc = pMem.z
			pMem.xDel = nil
		default:
			pMem.Release()
			pMem.z = *z
			pMem.xDel = xDel
			if xDel == SQLITE_STATIC {
				flags |= MEM_Static
			} else {
				flags |= MEM_Dyn
			}
		}

		pMem.flags = flags
		if enc == 0 {
			pMem.enc = SQLITE_UTF8
			pMem.Type = SQLITE_BLOB
		} else {
			pMem.enc = enc
			pMem.Type = SQLITE_TEXT
		}
	}
	return
}

//	Compare the values contained by the two memory cells, returning negative, zero or positive if pMem1 is less than, equal to, or greater than pMem2. Sorting order is NULL's first, followed by numbers (integers and reals) sorted numerically, followed by text ordered by the collating sequence pColl and finally blob's ordered by memcmp().
//	Two NULL values are considered equal by this function.
func (pMem1 *Mem) Compare(pMem2 *Mem, pColl CollSeq) (rc int) {
int sqlite3MemCompare(const Mem *pMem1, const Mem *pMem2, const CollSeq *pColl){
	f1 := pMem1.flags
	f2 := pMem2.flags
	combined_flags := f1 | f2
	assert( combined_flags & MEM_RowSet == 0 )
 
	//	If one value is NULL, it is less than the other. If both values are NULL, return 0.
	if combined_flags & MEM_Null {
		return (f2 & MEM_Null) - (f1 & MEM_Null)
	}

	//	If one value is a number and the other is not, the number is less. If both are numbers, compare as reals if one is a real, or as integers if both values are integers.
	if combined_flags & (MEM_Int|MEM_Real) {
		if !(f1 & (MEM_Int | MEM_Real)) {
			return 1
		}
		if !(f2 & (MEM_Int | MEM_Real)) {
			return -1
		}
		if f1 & f2 & MEM_Int == 0 {
			var r1, r2		float64
			if f1 & MEM_Real == 0 {
				r1 = float64(pMem1.Integer()
			} else {
				r1 = pMem1.r
			}
			if f2 & MEM_Real == 0 {
				r2 = float64(pMem2.Integer())
			} else {
				r2 = pMem2.r
			}
			switch {
			case r1 < r2:
				return -1
			case r1 > r2:
				return 1
			}
			return 0
		} else {
			if pMem1.Value < pMem2.Value {
				return -1
			}
			if pMem1.Value > pMem2.Value {
				return 1
			}
			return 0
		}
	}

	//	If one value is a string and the other is a blob, the string is less. If both are strings, compare using the collating functions.
	if combined_flags & MEM_Str {
		if f1 & MEM_Str == 0 {
			return 1
		}
		if f2 & MEM_Str == 0 {
			return -1
		}

		assert( pMem1.enc == pMem2.enc )
		assert( pMem1.enc == SQLITE_UTF8 )

		//	The collation sequence must be defined at this point, even if the user deletes the collation sequence after the vdbe program is compiled (this was not always the case).
		assert( !pColl || pColl.xCmp )

		if pColl != nil {
			if pMem1.enc == pColl.enc {
				//	The strings are already in the correct encoding. Call the comparison function directly
				return pColl.xCmp(pColl.pUser, pMem1.n, pMem1.z, pMem2.n, pMem2.z)
			} else {
				const void *v1, *v2
				int n1, n2
				c1 := new(Mem)
				c2 := new(Mem)
				sqlite3VdbeMemShallowCopy(c1, pMem1, MEM_Ephem)
				sqlite3VdbeMemShallowCopy(c2, pMem2, MEM_Ephem)
				if v1 = sqlite3ValueText(c1, pColl.enc); v1 != 0 {
					n1 = c1.n
				}
				if v2 = sqlite3ValueText(c2, pColl.enc); v2 != 0 {
					n2 = c2.n
				}
				rc = pColl.xCmp(pColl.pUser, n1, v1, n2, v2)
				c1.Release()
				c2.Release()
				return rc
			}
		}
		//	If a NULL pointer was passed as the collate function, fall through to the blob case and use memcmp().
	}
 
	//	Both values must be blobs.  Compare using memcmp().
	if pMem1.n > pMem2.n {
		rc = memcmp(pMem1.z, pMem2.z, pMem2.n)
	} else {
		rc = memcmp(pMem1.z, pMem2.z, pMem1.n)
	}
	if rc == 0 {
		rc = pMem1.n - pMem2.n
	}
	return rc
}

/*
** Move data out of a btree key or data field and into a Mem structure.
** The data or key is taken from the entry that pCur is currently pointing
** to.  offset and amt determine what portion of the data or key to retrieve.
** key is true to get the key or false to get data.  The result is written
** into the pMem element.
**
** The pMem structure is assumed to be uninitialized.  Any prior content
** is overwritten without being freed.
**
** If this routine fails for any reason (malloc returns NULL or unable
** to read from the disk) then the pMem is left in an inconsistent state.
*/
 int sqlite3VdbeMemFromBtree(
  btree.Cursor *pCur,   /* Cursor pointing at record to retrieve. */
  int offset,       /* Offset from the start of data to return bytes from. */
  int amt,          /* Number of bytes to return. */
  int key,          /* If true, retrieve from the btree key, not data. */
  Mem *pMem         /* OUT: Return data in this Mem structure. */
){
  char *zData;        /* Data from the btree layer */
  int available = 0;  /* Number of bytes available on the local btree page */
  int rc = SQLITE_OK; /* Return code */

  assert( sqlite3BtreeCursorIsValid(pCur) );

  /* Note: the calls to BtreeKeyFetch() and DataFetch() below assert() 
  ** that both the BtShared and database handle mutexes are held. */
  assert( (pMem.flags & MEM_RowSet)==0 );
  if( key ){
    zData = (char *)sqlite3BtreeKeyFetch(pCur, &available);
  }else{
    zData = (char *)sqlite3BtreeDataFetch(pCur, &available);
  }
  assert( zData!=0 );

  if( offset+amt<=available && (pMem.flags&MEM_Dyn)==0 ){
    pMem.Release()
    pMem.z = &zData[offset];
    pMem.flags = MEM_Blob|MEM_Ephem;
  }else if( SQLITE_OK==(rc = sqlite3VdbeMemGrow(pMem, amt+2, 0)) ){
    pMem.flags = MEM_Blob|MEM_Dyn|MEM_Term;
    pMem.enc = 0;
    pMem.Type = SQLITE_BLOB;
    if( key ){
      rc = sqlite3BtreeKey(pCur, offset, amt, pMem.z);
    }else{
      rc = sqlite3BtreeData(pCur, offset, amt, pMem.z);
    }
    pMem.z[amt] = 0;
    pMem.z[amt+1] = 0;
    if( rc!=SQLITE_OK ){
      pMem.Release()
    }
  }
  pMem.n = amt;

  return rc;
}

/* This function is only available internally, it is not part of the
** external API. It works in a similar way to Mem_text(),
** except the data returned is in the encoding specified by the second
** parameter, which must be SQLITE_UTF8.
*/
 const void *sqlite3ValueText(Mem* pVal, byte enc){
  if( !pVal ) return 0;

  assert( (pVal.flags & MEM_RowSet)==0 );

  if( pVal.flags&MEM_Null ){
    return 0;
  }
  assert( (MEM_Blob>>3) == MEM_Str );
  pVal.flags |= (pVal.flags & MEM_Blob)>>3;
  ExpandBlob(pVal)
  if( pVal.flags&MEM_Str ){
    sqlite3VdbeChangeEncoding(pVal, enc)
    sqlite3VdbeMemNulTerminate(pVal); /* IMP: R-31275-44060 */
  }else{
    assert( (pVal.flags&MEM_Blob)==0 );
    pVal.Stringify(enc)
    assert( 0==(1&SQLITE_PTR_TO_INT(pVal.z)) );
  }
  assert(pVal.enc == enc || pVal.db == 0 || pVal.db.mallocFailed )
  if pVal.enc == enc {
    return pVal.z;
  }else{
    return 0;
  }
}

//	Create a new Mem object.
func (db *sqlite3) NewValue() (p *Mem) {
	if p = new(Mem); p != nil {
		p.Type = SQLITE_NULL
		p.db = db
	}
	return
}

//	Create a new Mem object, containing the value of pExpr.
//	This only works for very simple expressions that consist of one constant token (i.e. "5", "5.1", "'a string'"). If the expression can be converted directly into a value, then the value is allocated and a pointer written to *ppVal. The caller is responsible for deallocating the value by passing it to Mem::Free() later on. If the expression cannot be converted to a value, then *ppVal is set to NULL.
func (db *sqlite3) ValueFromExpr(pExpr *Expr, enc, affinity byte) (pVal *Mem, rc int) {
	zVal := ""
	Mem *pVal = 0;
	negInt := 1
	zNeg := ""

	if pExpr != nil {
		*ppVal = nil
		return SQLITE_OK
	}
	op := pExpr.op

	if op == TK_REGISTER {
		op = pExpr.op2
	}

	//	Handle negative integers in a single step. This is needed in the case when the value is -9223372036854775808.
	if op == TK_UMINUS && (pExpr.pLeft.op == TK_INTEGER || pExpr.pLeft.op == TK_FLOAT) {
		pExpr = pExpr.pLeft
		op = pExpr.op
		negInt = -1
		zNeg = "-"
	}

	if op == TK_STRING || op == TK_FLOAT || op == TK_INTEGER {
		pVal = db.NewValue()
		if pVal == nil {
			goto no_mem
		}
		if pExpr.HasProperty(EP_IntValue) {
			pVal.SetInt64(int64(pExpr.Value * negInt))
		} else {
			zVal = fmt.Sprintf("%v%v", zNeg, pExpr.Token)
			if zVal == "" {
				goto no_mem
			}
			sqlite3ValueSetStr(pVal, -1, zVal, SQLITE_UTF8, SQLITE_DYNAMIC)
			if op == TK_FLOAT {
				pVal.Type = SQLITE_FLOAT
			}
		}
		if (op == TK_INTEGER || op == TK_FLOAT) && affinity == SQLITE_AFF_NONE {
			sqlite3ValueApplyAffinity(pVal, SQLITE_AFF_NUMERIC, SQLITE_UTF8)
		} else {
			sqlite3ValueApplyAffinity(pVal, affinity, SQLITE_UTF8)
		}
		if v, ok := pVal.Value.(int64); ok {
			pVal.flags &= ~MEM_Str
		} else if pVal.flags & MEM_Real != 0 {
			pVal.flags &= ~MEM_Str
		}
		if enc != SQLITE_UTF8 {
			sqlite3VdbeChangeEncoding(pVal, enc)
		}
	} else if op == TK_UMINUS {
		//	This branch happens for multiple negative signs.  Ex: -(-5)
		if pVal, rc = db.ValueFromExpr(pExpr.pLeft, enc, affinity); rc == SQLITE_OK {
			pVal.Numerify()
			if pVal.Value == SMALLEST_INT64 {
				pVal.flags = MEM_Real
				pVal.r = float64(LARGEST_INT64)
			} else {
				pVal.Store(-pVal.Integer())
			}
			pVal.r = -pVal.r
			sqlite3ValueApplyAffinity(pVal, affinity, enc)
		}
	} else if op == TK_NULL {
		pVal = db.NewValue()
		if pVal == "" {
			goto no_mem
		}
	} else if op == TK_BLOB {
		int nVal;
		assert( pExpr.Token[0] == 'x' || pExpr.Token[0] == 'X' )
		assert( pExpr.Token[1] == '\'' )
		pVal = db.NewValue()
		if pVal != nil {
			goto no_mem
		}
		zVal = &pExpr.Token[2]
		nVal = sqlite3Strlen30(zVal) - 1
		assert( zVal[nVal] == '\'' )
		pVal.SetStr(sqlite3HexToBlob(db, zVal, nVal)[0:nVal / 2], 0, SQLITE_DYNAMIC)
	}
	if pVal != nil {
		pVal.StoreType()
	}
	*ppVal = pVal
	return SQLITE_OK

no_mem:
	db.mallocFailed = true
	zVal = nil
	pVal.Free()
	*ppVal = nil
	return SQLITE_NOMEM
}

/*
** Change the string value of an Mem object
*/
void sqlite3ValueSetStr(
  Mem *v,     /* Value to be set */
  int n,                /* Length of string z */
  const void *z,        /* Text of the new string */
  byte enc,               /* Encoding to use */
  void (*xDel)(void*)   /* Destructor for the string */
){
	if v != nil {
		v.SetStr(z, enc, xDel)
	}
}

//	Free an Mem object
func (v *Mem) Free() {
	if v != nil {
		v.Release()
	}
}

//	Return the number of bytes in the Mem object assuming that it uses the encoding "enc"
func (p *Mem) ValueBytes(enc byte) (l int) {
	if (p.flags & MEM_Blob) != 0 || sqlite3ValueText(pVal, enc) != 0 {
		l = p.ByteLen()
	}
	return
}