import "crypto/rand"

/* This file contains code used for creating, destroying, and populating
** a VDBE (or an "sqlite3_stmt" as it is known to the outside world.)  Prior
** to version 2.8.7, all this code was combined into the vdbe.c source file.
** But that file was getting too big so this subroutines were split out.
*/

/*
** Create a new virtual database engine.
*/
 Vdbe *sqlite3VdbeCreate(sqlite3 *db){
  Vdbe *p;
  p = sqlite3DbMallocZero(db, sizeof(Vdbe) );
  if( p==0 ) return 0;
  p.db = db;
  if( db.pVdbe ){
    db.pVdbe.pPrev = p;
  }
  p.Next = db.pVdbe;
  p.pPrev = 0;
  db.pVdbe = p;
  p.magic = VDBE_MAGIC_INIT;
  return p;
}

func (v *Vdbe) Comment(args... interface{}) {}
func (v *Vdbe) NoopComment(args... interface{}) {}

/*
** Remember the SQL string for a prepared statement.
*/
 void sqlite3VdbeSetSql(Vdbe *p, const char *z, int n, int isPrepareV2){
  assert( isPrepareV2==1 || isPrepareV2==0 );
  if( p==0 ) return;
#ifdef SQLITE_OMIT_TRACE
  if( !isPrepareV2 ) return;
#endif
  assert( p.zSql==0 );
  p.zSql = sqlite3DbStrNDup(p.db, z, n);
  p.isPrepareV2 = (byte)isPrepareV2;
}

/*
** Return the SQL associated with a prepared statement
*/
func const char *sqlite3_sql(sqlite3_stmt *pStmt){
  Vdbe *p = (Vdbe *)pStmt;
  return (p && p.isPrepareV2) ? p.zSql : 0;
}

/*
** Swap all content between two VDBE structures.
*/
 void sqlite3VdbeSwap(Vdbe *pA, Vdbe *pB){
  Vdbe tmp, *pTmp;
  char *zTmp;
  tmp = *pA;
  *pA = *pB;
  *pB = tmp;
  pTmp = pA.Next;
  pA.Next = pB.Next;
  pB.Next = pTmp;
  pTmp = pA.pPrev;
  pA.pPrev = pB.pPrev;
  pB.pPrev = pTmp;
  zTmp = pA.zSql;
  pA.zSql = pB.zSql;
  pB.zSql = zTmp;
  pB.isPrepareV2 = pA.isPrepareV2;
}

//	Add a new instruction to the list of instructions current in the VDBE. Return the address of the new instruction.
//	Parameters:
//		p               Pointer to the VDBE
//		op              The opcode for this instruction
//		p1, p2, p3      Operands
//	Use the Vdbe::ResolveLabel() function to fix an address and the sqlite3VdbeChangeP4() function to change the value of the P4 operand.
func (p *Vdbe) AddOp3(op, p1, p2, p3 int) (addr int) {
	assert( p.magic == VDBE_MAGIC_INIT )
	assert( op > 0 && op < 0xff )
	addr = len(p.Program)
	p.Program = append(p.Program, &Op{ opcode: byte(op), p1: p1, p2: p2, p3: p3, p4type: P4_NOTUSED })
	return
}

func (p *Vdbe) AddOp0(op int) int {
	return p.AddOp3(op, 0, 0, 0)
}

func (p* Vdbe) AddOp1(op, p1 int) int {
	 return p.AddOp3(op, p1, 0, 0)
}

func (p *Vdbe) VdbeAddOp2(op, p1, p2 int) {
	return p.AddOp3(op, p1, p2, 0)
}

//	Add an opcode that includes the p4 value as a pointer.
int sqlite3VdbeAddOp4(
  Vdbe *p,            /* Add the opcode to this VM */
  int op,             /* The new opcode */
  int p1,             /* The P1 operand */
  int p2,             /* The P2 operand */
  int p3,             /* The P3 operand */
  const char *zP4,    /* The P4 operand */
  int p4type          /* P4 operand type */
){
  int addr = p.AddOp3(op, p1, p2, p3);
  sqlite3VdbeChangeP4(p, addr, zP4, p4type);
  return addr;
}

//	Add an OP_ParseSchema opcode. This routine is broken out from sqlite3VdbeAddOp4() since it needs to also mark all btrees as having been used.
func (p *Vdbe) AddParseSchemaOp(iDb int, zWhere string) {
	addr := p.AddOp3(OP_ParseSchema, iDb, 0, 0)
	sqlite3VdbeChangeP4(p, addr, zWhere, P4_DYNAMIC)
	for i, _ := range p.db.Databases {
		sqlite3VdbeUsesBtree(p, i)
	}
}

/*
** Add an opcode that includes the p4 value as an integer.
*/
 int sqlite3VdbeAddOp4Int(
  Vdbe *p,            /* Add the opcode to this VM */
  int op,             /* The new opcode */
  int p1,             /* The P1 operand */
  int p2,             /* The P2 operand */
  int p3,             /* The P3 operand */
  int p4              /* The P4 operand as an integer */
){
  int addr = p.AddOp3(op, p1, p2, p3)
  sqlite3VdbeChangeP4(p, addr, SQLITE_INT_TO_PTR(p4), P4_INT32);
  return addr;
}

//	Create a new symbolic label for an instruction that has yet to be coded. The symbolic label is really just a negative number. The label can be used as the P2 value of an operation. Later, when the label is resolved to a specific address, the VDBE will scan through its operation list and change all values of P2 which match the label into the resolved address.
//	The VDBE knows that a P2 value is a label because labels are always negative and P2 values are suppose to be non-negative. Hence, a negative P2 value is a label that has yet to be resolved.
//	Zero is returned if a malloc() fails.
func (p *Vdbe) MakeLabel() (i int) {
	i = p.nLabel
	p.nLabel++
	assert( p.magic == VDBE_MAGIC_INIT )
	if (i & (i - 1)) == 0 {
		p.aLabel = sqlite3DbReallocOrFree(p.db, p.aLabel, (i * 2 + 1) *sizeof(p.aLabel[0]))
	}
	if p.aLabel != nil {
		p.aLabel[i] = -1
	}
	return -1 - i
}

//	Resolve label "x" to be the address of the next instruction to be inserted. The parameter "x" must have been obtained from a prior call to MakeLabel().
func (p *Vdbe) ResolveLabel(x int) {
	j := -1 - x
	assert( p.magic == VDBE_MAGIC_INIT )
	assert( j >= 0 && j < p.nLabel )
	if p.aLabel != nil {
		p.aLabel[j] = len(p.Program)
	}
}

//	Mark the VDBE as one that can only be run one time.
func (p *Vdbe) RunOnlyOnce() {
	p.runOnlyOnce = true
}


//	Loop through the program looking for P2 values that are negative on jump instructions. Each such value is a label. Resolve the label by setting the P2 value to its correct non-zero value.
//	This routine is called once after all opcodes have been inserted.
//	Variable *pMaxFuncArgs is set to the maximum value of any P2 argument to an OP_Function, OP_AggStep or OP_VFilter opcode. This is used by sqlite3VdbeMakeReady() to size the Vdbe.apArg[] array.
//	The Op.opflags field is set on all opcodes.
func (p *Vdbe) resolveP2Values(nMaxArgs int) int {
	aLabel = p.aLabel
	p.readOnly = true
	for _, pOp := range p.Program {
		opcode := pOp.opcode
		pOp.opflags = OPCODE_PROPERTIES[opcode]
		switch {
		case opcode == OP_Function || opcode == OP_AggStep:
			if pOp.p5 > nMaxArgs {
				nMaxArgs = pOp.p5
			}
		case (opcode == OP_Transaction && pOp.p2 != 0) || opcode == OP_Vacuum:
			p.readOnly = false
		case opcode == OP_VUpdate:
			if pOp.p2 > nMaxArgs {
				nMaxArgs = pOp.p2
			}
		case opcode == OP_VFilter:
			assert( pOp[-1].opcode == OP_Integer )
			n := pOp[-1].p1;
			if n > nMaxArgs {
				nMaxArgs = n
			}
		case opcode == OP_Next || opcode == OP_SorterNext:
			pOp.p4.xAdvance = sqlite3BtreeNext
			pOp.p4type = P4_ADVANCE
		case opcode == OP_Prev:
			pOp.p4.xAdvance = sqlite3BtreePrevious
			pOp.p4type = P4_ADVANCE
		}

		if pOp.opflags & OPFLG_JUMP != 0 && pOp.p2 < 0 {
			assert( -1 - pOp.p2 < p.nLabel )
			pOp.p2 = aLabel[-1 - pOp.p2]
		}
	}
	p.aLabel = nil
	return nMaxArgs
}

//	Return the address of the next instruction to be inserted.
func (p *Vdbe) CurrentAddr() int {
	assert( p.magic==VDBE_MAGIC_INIT )
	return len(p.Program)
}

//	This function returns a pointer to the array of opcodes associated with the Vdbe passed as the first argument. It is the callers responsibility to arrange for the returned array to be eventually freed using the vdbe::FreeOpArray() function.
//	Before returning, maxFuncArgs is set to the larger of nMaxArg and the number of entries in the Vdbe.apArg[] array required to execute the returned program.
func (p *Vdbe) TakeOpArray(nMaxArg int) (program []VdbeOp, maxFuncArgs int) {
	program := p.Program
	assert( program != nil && !p.db.mallocFailed )

	//	Check that sqlite3VdbeUsesBtree() was not called on this VM
	assert( !p.btreeMask )
	maxFuncArgs = p.resolveP2Values(nMaxArg)
	p.Program = nil
	return
}

//	Add a whole list of operations to the operation stack. Return the address of the first operation added.
func (p *Vdbe) AddOpList(ops... VdbeOpList) (addr int) {
	assert( p.magic == VDBE_MAGIC_INIT )
	addr = len(p.Program)
	for i, op := range ops {
		pOut := &Op{ opcode: op.opcode, p1: op.p1, p3: op.p3, p4type: P4_NOTUSED }
		if p2 := op.p2; p2 < 0 && (OPCODE_PROPERTIES[pOut.opcode] & OPFLG_JUMP) != 0 {
			pOut.p2 = addr + ADDR(p2)
		} else {
			pOut.p2 = p2
		}
		p.Program = append(p.Program, pOut)
	}
	return
}

//	Change the value of the P1 operand for a specific instruction.
//	This routine is useful when a large program is loaded from a static array using Vdbe::AddOpList but we want to make a few minor changes to the program.
func (p *Vdbe) ChangeP1(addr uint32, val int) {
	if uint32(len(p.Program)) > addr {
		p.Program[addr].p1 = val
	}
}

//	Change the value of the P2 operand for a specific instruction.
//	This routine is useful for setting a jump destination.
func (p *Vdbe) ChangeP2(addr uint32, val int) {
	if uint32(len(p.Program)) > addr {
		p.Program[addr].p2 = val
	}
}

//	Change the value of the P3 operand for a specific instruction.
func (p *Vdbe) ChangeP3(addr uint32, val int) {
	if uint32(len(p.Program)) > addr {
		p.Program[addr].p3 = val
	}
}

//	Change the value of the P5 operand for the most recently added operation.
func (p *Vdbe) ChangeP5(val byte) {
	if len(p.Program) > 0 {
		p.Program[len(p.Program) - 1].p5 = val
	}
}

//	Change the P2 operand of instruction addr so that it points to the address of the next instruction to be coded.
func (p *Vdbe) JumpHere(addr int) {
	assert( addr >= 0 || p.db.mallocFailed )
	if addr >= 0 {
		p.ChangeP2(addr, len(p.Program))
	}
}


//	If the input FuncDef structure is ephemeral, then free it. If the FuncDef is not ephermal, then do nothing.
func (db *sqlite3) freeEphemeralFunction(pDef *FuncDef) {
	if pDef != nil && (pDef.flags & SQLITE_FUNC_EPHEM) != 0 {
		pDef = nil
	}
}

//	Delete a P4 value if necessary.
func (db *sqlite3) freeP4(p4type int, p4 interface{}) {
	if p4 != nil {
		assert( db != nil )
		switch p4type {
		case P4_REAL, P4_INT64, P4_DYNAMIC, P4_KEYINFO, P4_INTARRAY, P4_KEYINFO_HANDOFF:
			p4 = nil
		case P4_MPRINTF:
			if db.pnBytesFreed == 0 {
				p4 = nil
			}
		case P4_VDBEFUNC:
			pVdbeFunc := (VdbeFunc *)(p4)
			db.freeEphemeralFunction(pVdbeFunc.pFunc)
			if db.pnBytesFreed == 0 {
				sqlite3VdbeDeleteAuxData(pVdbeFunc, 0)
			}
			pVdbeFunc = nil
		case P4_FUNCDEF:
			db.freeEphemeralFunction((FuncDef*)(p4))
		case P4_MEM:
			if db.pnBytesFreed == 0 {
				p4.Free()
			} else {
				p := (Mem*)(p4)
				p.zMalloc = nil
				p = nil
			}
		case P4_VTAB:
			if db.pnBytesFreed == 0 {
				(VTable *)(p4).Unlock()
			}
		}
	}
}

//	Free the space allocated for program and any p4 values allocated for the opcodes contained within.
func (db *sqlite3) FreeOpArray(program *[]Op) {
	if program != nil {
		for _, op := range program {
			db.freeP4(op.p4type, op.p4.p)
		}
	}
	*program = nil
}

//	Link the SubProgram object passed as the second argument into the linked list at Vdbe.pSubProgram. This list is used to delete all sub-program objects when the VM is no longer required.
func (p *Vdbe) LinkRoutine(s *SubProgram){
	p.Routines = append(p.Routines, s)
}

//	Change the opcode at addr into OP_Noop
func (p *Vdbe) ChangeToNoop(addr int) {
	if p.Program != nil {
		pOp := &p.Program[addr]
		db := p.db
		db.freeP4(pOp.p4type, pOp.p4.p)
		memset(pOp, 0, sizeof(pOp[0]))
		pOp.opcode = OP_Noop
	}
}

//	Change the value of the P4 operand for a specific instruction. This routine is useful when a large program is loaded from a static array using Vdbe::AddOpList but we want to make a few minor changes to the program.
//	If n >= 0 then the P4 operand is dynamic, meaning that a copy of the string is made into memory obtained from sqlite3_malloc().
//	A value of n == 0 means copy bytes of zP4 up to and including the first null byte.
//	If n > 0 then copy n + 1 bytes of zP4.
//	If n == P4_KEYINFO it means that zP4 is a pointer to a KeyInfo structure. A copy is made of the KeyInfo structure into memory obtained from sqlite3_malloc, to be freed when the Vdbe is finalized.
//	n == P4_KEYINFO_HANDOFF indicates that zP4 points to a KeyInfo structure stored in memory that the caller has obtained from sqlite3_malloc. The caller should not free the allocation, it will be freed when the Vdbe is finalized.
//	Other values of n (P4_STATIC, P4_COLLSEQ etc.) indicate that zP4 points to a string or structure that is guaranteed to exist for the lifetime of the Vdbe. In these cases we can just copy the pointer.
//	If addr < 0 then change P4 on the most recently inserted instruction.
 void sqlite3VdbeChangeP4(Vdbe *p, int addr, const char *zP4, int n){
  Op *pOp;
  sqlite3 *db;
  assert( p!=0 );
  db = p.db;
  assert( p.magic==VDBE_MAGIC_INIT );
  if( p.Program == nil || db.mallocFailed ){
    if ( n!=P4_KEYINFO && n!=P4_VTAB ) {
      db.freeP4(n, (void*)*(char**)&zP4);
    }
    return;
  }
  assert( len(p.Program) > 0 )
  assert( addr < len(p.Program) )
  if addr < 0 {
    addr = len(p.Program) - 1
  }
  pOp = &p.Program[addr];
  db.freeP4(pOp.p4type, pOp.p4.p)
  pOp.p4.p = 0;
  if( n==P4_INT32 ){
    /* Note: this cast is safe, because the origin data point was an int
    ** that was cast to a (const char *). */
    pOp.p4.i = SQLITE_PTR_TO_INT(zP4);
    pOp.p4type = P4_INT32;
  }else if( zP4==0 ){
    pOp.p4.p = 0;
    pOp.p4type = P4_NOTUSED;
  }else if( n==P4_KEYINFO ){
    KeyInfo *pKeyInfo;
    int nField, nByte;

    nField = ((KeyInfo*)zP4).nField;
    nByte = sizeof(*pKeyInfo) + (nField-1)*sizeof(pKeyInfo.Collations[0]) + nField;
    pKeyInfo = sqlite3DbMallocRaw(0, nByte);
    pOp.p4.pKeyInfo = pKeyInfo;
    if( pKeyInfo ){
      byte *aSortOrder;
      memcpy((char*)pKeyInfo, zP4, nByte - nField);
      aSortOrder = pKeyInfo.aSortOrder;
      if( aSortOrder ){
        pKeyInfo.aSortOrder = (unsigned char*)&pKeyInfo.Collations[nField];
        memcpy(pKeyInfo.aSortOrder, aSortOrder, nField);
      }
      pOp.p4type = P4_KEYINFO;
    }else{
      p.db.mallocFailed = true
      pOp.p4type = P4_NOTUSED;
    }
  }else if( n==P4_KEYINFO_HANDOFF ){
    pOp.p4.p = (void*)zP4;
    pOp.p4type = P4_KEYINFO;
  }else if( n==P4_VTAB ){
    pOp.p4.p = (void*)zP4;
    pOp.p4type = P4_VTAB;
    (VTable *)(zP4).Lock()
    assert( (VTable *)(zP4).db==p.db );
  }else if( n<0 ){
    pOp.p4.p = (void*)zP4;
    pOp.p4type = (signed char)n;
  }else{
    if( n==0 ) n = sqlite3Strlen30(zP4);
    pOp.p4.z = sqlite3DbStrNDup(p.db, zP4, n);
    pOp.p4type = P4_DYNAMIC;
  }
}

/*
** Return the opcode for a given address.  If the address is -1, then
** return the most recently inserted opcode.
**
** If a memory allocation error has occurred prior to the calling of this
** routine, then a pointer to a dummy VdbeOp will be returned.  That opcode
** is readable but not writable, though it is cast to a writable value.
** The return of a dummy opcode allows the call to continue functioning
** after a OOM fault without having to check to see if the return from 
** this routine is a valid pointer.  But because the dummy.opcode is 0,
** dummy will never be written to.  This is verified by code inspection and
** by running with Valgrind.
**
** About the #ifdef SQLITE_OMIT_TRACE:  Normally, this routine is never called
** unless len(p.Program) > 0.  This is because in the absense of SQLITE_OMIT_TRACE,
** an OP_Trace instruction is always inserted by sqlite3VdbeGet() as soon as
** a new VDBE is created.  So we are free to set addr to len(p.Program) - 1 without
** having to double-check to make sure that the result is non-negative. But
** if SQLITE_OMIT_TRACE is defined, the OP_Trace is omitted and we do need to
** check the value of len(p.Program) - 1 before continuing.
*/
 VdbeOp *sqlite3VdbeGetOp(Vdbe *p, int addr){
  /* C89 specifies that the constant "dummy" will be initialized to all
  ** zeros, which is correct.  MSVC generates a warning, nevertheless. */
  static VdbeOp dummy;  /* Ignore the MSVC warning about no initializer */
  assert( p.magic==VDBE_MAGIC_INIT );
  if( addr<0 ){
#ifdef SQLITE_OMIT_TRACE
    if len(p.Program) == 0 {
		return (VdbeOp*)&dummy
	}
#endif
    addr = len(p.Program) - 1
  }
  assert( (addr >= 0 && addr < len(p.Program)) || p.db.mallocFailed )
  if( p.db.mallocFailed ){
    return (VdbeOp*)&dummy;
  }else{
    return &p.Program[addr];
  }
}

#if !defined(SQLITE_OMIT_EXPLAIN) || defined(VDBE_PROFILE)
/*
** Compute a string that describes the P4 parameter for an opcode.
** Use zTemp for any required temporary buffer space.
*/
static char *displayP4(Op *pOp, char *zTemp, int nTemp){
  char *zP4 = zTemp;
  assert( nTemp>=20 );
  switch( pOp.p4type ){
    case P4_KEYINFO_STATIC:
    case P4_KEYINFO: {
      int i, j;
      KeyInfo *pKeyInfo = pOp.p4.pKeyInfo;
      zTemp = fmt.Sprintf("keyinfo(%d", pKeyInfo.nField);
      i = sqlite3Strlen30(zTemp);
      for(j=0; j<pKeyInfo.nField; j++){
        CollSeq *pColl = pKeyInfo.Collations[j];
        if( pColl ){
          int n = sqlite3Strlen30(pColl.Name);
          if( i+n>nTemp-6 ){
            memcpy(&zTemp[i],",...",4);
            break;
          }
          zTemp[i++] = ',';
          if( pKeyInfo.aSortOrder && pKeyInfo.aSortOrder[j] ){
            zTemp[i++] = '-';
          }
          memcpy(&zTemp[i], pColl.Name,n+1);
          i += n;
        }else if( i+4<nTemp-6 ){
          memcpy(&zTemp[i],",nil",4);
          i += 4;
        }
      }
      zTemp[i++] = ')';
      zTemp[i] = 0;
      assert( i<nTemp );
      break;
    }
    case P4_COLLSEQ: {
      CollSeq *pColl = pOp.p4.pColl;
      zTemp = fmt.Sprintf("collseq(%.20s)", pColl.Name);
      break;
    }
    case P4_FUNCDEF: {
      FuncDef *pDef = pOp.p4.pFunc;
      zTemp = fmt.Sprintf("%s(%d)", pDef.Name, pDef.nArg);
      break;
    }
    case P4_INT64: {
      zTemp = fmt.Sprintf("%lld", *pOp.p4.pI64);
      break;
    }
    case P4_INT32: {
      zTemp = fmt.Sprintf("%d", pOp.p4.i);
      break;
    }
    case P4_REAL: {
      zTemp = fmt.Sprintf("%.16g", *pOp.p4.pReal);
      break;
    }
    case P4_MEM:
		switch v := pOp.p4.pMem.Value.(type); {
		case pMem.flags & MEM_Str != 0:
			zP4 = pMem.z
		case int64:
			zTemp = fmt.Sprintf("%lld", v)
		case pMem.flags & MEM_Real != 0:
			zTemp = fmt.Sprintf("%.16g", pMem.r)
		case pMem.flags & MEM_Null != 0:
			zTemp = "NULL"
		default:
			assert( pMem.flags & MEM_Blob != 0 )
			zP4 = "(blob)"
		}
    case P4_VTAB: {
      sqlite3_vtab *pVtab = pOp.p4.pVtab.pVtab;
      zTemp = fmt.Sprintf("vtab:%v:%v", pVtab, pVtab.Callbacks);
      break;
    }
    case P4_INTARRAY: {
      zTemp = "intarray"
      break;
    }
    case P4_SUBPROGRAM: {
      zTemp = "program"
      break;
    }
    case P4_ADVANCE: {
      zTemp[0] = 0;
      break;
    }
    default: {
      zP4 = pOp.p4.z;
      if( zP4==0 ){
        zP4 = zTemp;
        zTemp[0] = 0;
      }
    }
  }
  assert( zP4!=0 );
  return zP4;
}
#endif

/*
** Declare to the Vdbe that the BTree object at db.Databases[i] is used.
**
** The prepared statements need to know in advance the complete set of
** attached databases that will be use.  A mask of these databases
** is maintained in p.btreeMask.  The p.lockMask value is the subset of
** p.btreeMask of databases that will require a lock.
*/
 void sqlite3VdbeUsesBtree(Vdbe *p, int i){
  assert( i >= 0 && i < len(p.db.Databases) && i < int(sizeof(yDbMask)) * 8 )
  assert( i < int(sizeof(p.btreeMask)) * 8 )
  p.btreeMask |= ((yDbMask)1)<<i;
  if i != 1 && p.db.Databases[i].pBt.BtreeShareable() {
    p.lockMask |= ((yDbMask)1)<<i;
  }
}

//	If SQLite is compiled to support shared-cache mode and to be threadsafe, this routine obtains the mutex associated with each BtShared structure
//	that may be accessed by the VM passed as an argument. In doing so it also sets the BtShared.db member of each of the BtShared structures, ensuring
//	that the correct busy-handler callback is invoked if required.
//
//	If SQLite is not threadsafe but does support shared-cache mode, then Lock() is invoked to set the BtShared.db variables
//	of all of BtShared structures accessible via the database handle associated with the VM.
//
//	The p.btreeMask field is a bitmask of all btrees that the prepared statement p will ever use. Let N be the number of bits in p.btreeMask
//	corresponding to btrees that use shared cache. Then the runtime of this routine is N*N. But as N is rarely more than 1, this should not be a problem.
func (p *Vdbe) Enter() {
	if p.lockMask != 0 {
		db := p.db
		mask := yDbMask(1)
		for i, database := range db.Databases {
			if i != 1 && (mask & p.lockMask) != 0 && database.pBt != nil {
				database.pBt.Lock()
			}
			mask += mask
		}
	}
}


//	Unlock all of the btrees previously locked by a call to Enter().
func (p *Vdbe) Leave() {
	if p.lockMask != 0 {
		db := p.db
		mask := yDbMask(1)
		for i, database := range db.Databases {
			if i != 1 && (mask & p.lockMask) != 0 && database.pBt != nil {
				database.pBt.Unlock()
			}
			mask += mask
		}
	}
}

#if defined(VDBE_PROFILE)
/*
** Print a single opcode.  This routine is used for debugging only.
*/
 void sqlite3VdbePrintOp(FILE *pOut, int pc, Op *pOp){
  char *zP4;
  char zPtr[50];
  static const char *zFormat1 = "%4d %-13s %4d %4d %4d %-4s %.2X %s\n";
  if( pOut==0 ) pOut = stdout;
  zP4 = displayP4(pOp, zPtr, sizeof(zPtr));
  fprintf(pOut, zFormat1, pc, 
      sqlite3OpcodeName(pOp.opcode), pOp.p1, pOp.p2, pOp.p3, zP4, pOp.p5, "");
  fflush(pOut);
}
#endif

//	Delete a VdbeFrame object and its contents. VdbeFrame objects are allocated by the OP_Program opcode in sqlite3VdbeExec().
func (p *VdbeFrame) Delete() {
	aMem := VdbeFrameMem(p)
	apCsr := (VdbeCursor **)(&aMem[p.nChildMem])
	for i := 0; i < p.nChildCsr; i++ {
		p.v.FreeCursor(apCsr[i])
	}
	p = nil
}

#ifndef SQLITE_OMIT_EXPLAIN
//	Give a listing of the program in the virtual machine.
//	The interface is the same as sqlite3VdbeExec(). But instead of running the code, it invokes the callback once for each instruction.
//	This feature is used to implement "EXPLAIN".
//	When p.explain==1, each instruction is listed. When p.explain==2, only OP_Explain instructions are listed and these are shown in a different format. p.explain==2 is used to implement EXPLAIN QUERY PLAN.
//	When p.explain==1, first the main program is listed, then each of the trigger subprograms are listed one by one.
func (p *Vdbe) List() (rc int) {
	db := p.db
	apSub := []*SubProgram
	rc = SQLITE_OK

	assert( p.explain )
	assert( p.magic == VDBE_MAGIC_RUN )
	assert( p.rc == SQLITE_OK || p.rc == SQLITE_BUSY || p.rc == SQLITE_NOMEM )

	//	Even though this opcode does not use dynamic strings for the result, result columns may become dynamic if the user calls sqlite3_column_text16(), causing a translation to UTF-16 encoding.
	//  releaseMemArray(pMem, 8)
	p.pResultSet = nil

	if p.rc == SQLITE_NOMEM {
		//	This happens if a malloc() inside a call to sqlite3_column_text() or sqlite3_column_text16() failed.
		db.mallocFailed = true
		return SQLITE_ERROR
	}

	//	When the number of output rows reaches nRow, that means the listing has finished and sqlite3_step() should return SQLITE_DONE. nRow is the sum of the number of rows in the main program, plus the sum of the number of rows in all trigger subprograms encountered so far. The nRow value will increase as new trigger subprograms are encountered, but p.pc will eventually catch up to nRow.
	pSub	*Mem
	nRow := len(p.Program)
	if p.explain == 1 {
		//	The first 8 memory cells are used for the result set. So we will commandeer the 9th cell to use as storage for an array of pointers to trigger subprograms. The VDBE is guaranteed to have at least 9 cells.
		assert( p.nMem > 9 )
		pSub = &p.aMem[9]
		if pSub.flags&MEM_Blob {
			//	On the first call to sqlite3_step(), pSub will hold a NULL. It is initialized to a BLOB by the P4_SUBPROGRAM processing logic below
			nSub = pSub.n / sizeof(Vdbe*)
			apSub = (SubProgram **)pSub.z
		}
		for i := 0; i < nSub; i++ {
			nRow += len(apSub[i])
		}
	}

	i := 0
	//	WTF!!!!!
	do{
		i = p.pc++
	}while( i < nRow && p.explain == 2 && p.Program[i].opcode != OP_Explain )
	if i >= nRow {
		p.rc = SQLITE_OK
		rc = SQLITE_DONE
	} else if db.u1.isInterrupted {
		p.rc = SQLITE_INTERRUPT
		rc = SQLITE_ERROR
		p.zErrMsg = fmt.Sprintf("%v", sqlite3ErrStr(p.rc))
	} else {
		char *z
		Op *pOp;

		if i < len(p.Program) {
			//	The output line number is small enough that we are still in the main program.
			pOp = p.Program[i]
		} else {
			//	We are currently listing subprograms. Figure out which one and pick up the appropriate opcode.
			int j
			i -= len(p.Program)
			for j = 0; i >= len(apSub[j].Opcodes); j++ {
				i -= len(apSub[j].Opcodes)
			}
			pOp = apSub[j].Opcodes[i]
		}

		pMem := p.aMem[1]											//	First Mem of result set
		if p.explain == 1 {
			pMem.Type = SQLITE_INTEGER
			pMem.Store(int64(i))									//	Program counter
			pMem++
  
			pMem.flags = MEM_Static | MEM_Str | MEM_Term
			pMem.z = (char*)sqlite3OpcodeName(pOp.opcode)			//	Opcode
			assert( pMem.z != 0 )
			pMem.n = sqlite3Strlen30(pMem.z)
			pMem.Type = SQLITE_TEXT
			pMem.enc = SQLITE_UTF8
			pMem++

			//	When an OP_Program opcode is encounter (the only opcode that has a P4_SUBPROGRAM argument), expand the size of the array of subprograms kept in p.aMem[9].z to hold the new program - assuming this subprogram has not already been seen.
			if pOp.p4type == P4_SUBPROGRAM {
				int nByte = (nSub+1)*sizeof(SubProgram*)
				int j;
				for j = 0; j < nSub; j++ {
					if apSub[j] == pOp.p4.SubProgram {
						break
					}
				}
				if j == nSub && SQLITE_OK == sqlite3VdbeMemGrow(pSub, nByte, nSub != 0) {
					apSub = (SubProgram **)(pSub.z)
					apSub = append(apSub, pOp.p4.SubProgram)
					pSub.flags |= MEM_Blob
					pSub.n = len(apSub) * sizeof(SubProgram*)
				}
			}
		}

		pMem.Store(int64(pOp.p1))
		pMem.Type = SQLITE_INTEGER
		pMem++

		pMem.Store(int64(pOp.p2))
		pMem.Type = SQLITE_INTEGER
		pMem++

		pMem.Store(int64(pOp.p3))
		pMem.Type = SQLITE_INTEGER
		pMem++

		if sqlite3VdbeMemGrow(pMem, 32, 0) {
			assert( p.db.mallocFailed )
			return SQLITE_ERROR
		}
		pMem.flags = MEM_Dyn | MEM_Str | MEM_Term
		if z = displayP4(pOp, pMem.z, 32); z != pMem.z {
			pMem.SetStr(z, SQLITE_UTF8, 0)
		} else {
			assert( pMem.z != 0 )
			pMem.n = sqlite3Strlen30(pMem.z)
			pMem.enc = SQLITE_UTF8
		}
		pMem.Type = SQLITE_TEXT
		pMem++

		if p.explain == 1 {
			if sqlite3VdbeMemGrow(pMem, 4, 0) {
				assert( p.db.mallocFailed )
				return SQLITE_ERROR
			}
			pMem.flags = MEM_Dyn | MEM_Str | MEM_Term
			pMem.n = 2
			pMem.z = fmt.Sprintf("%.2x", pOp.p5)
			pMem.Type = SQLITE_TEXT
			pMem.enc = SQLITE_UTF8
			pMem++
			pMem.flags = MEM_Null					//	Comment
			pMem.Type = SQLITE_NULL
		}
		p.nResColumn = 8 - 4*(p.explain-1)
		p.pResultSet = &p.aMem[1]
		p.rc = SQLITE_OK
		rc = SQLITE_ROW
	}
	return rc
}
#endif /* SQLITE_OMIT_EXPLAIN */

#if !defined(SQLITE_OMIT_TRACE) && defined(SQLITE_ENABLE_IOTRACE)
//	Print an IOTRACE message showing SQL content.
func (p *Vdbe) IOTraceSql() {
	if sqlite3IoTrace == nil || len(p.Program) == 0 {
		return
	}
	if op := &p.Program[0]; op.opcode == OP_Trace && op.p4.z != "" {
		z = pOp.p4.z
		i, j	int
		for i = 0; sqlite3Isspace(z[i]); i++ {}
		for j = 0; z[i]; i++ {
			if sqlite3Isspace(z[i]) {
				if z[i - 1] != ' ' {
					z[j] = ' '
					j++
				}
			} else {
				z[j] = z[i]
				j++
			}
		}
		z[j] = 0
		sqlite3IoTrace("SQL %s\n", z)
	}
}
#endif /* !SQLITE_OMIT_TRACE && SQLITE_ENABLE_IOTRACE */

/*
** Allocate space from a fixed size buffer and return a pointer to
** that space.  If insufficient space is available, return NULL.
**
** The pBuf parameter is the initial value of a pointer which will
** receive the new memory.  pBuf is normally NULL.  If pBuf is not
** NULL, it means that memory space has already been allocated and that
** this routine should not allocate any new memory.  When pBuf is not
** NULL simply return pBuf.  Only allocate new memory space when pBuf
** is NULL.
**
** nByte is the number of bytes of space needed.
**
** *ppFrom points to available space and pEnd points to the end of the
** available space.  When space is allocated, *ppFrom is advanced past
** the end of the allocated space.
**
** *pnByte is a counter of the number of bytes of space that have failed
** to allocate.  If there is insufficient space in *ppFrom to satisfy the
** request, then increment *pnByte by the amount of the request.
*/
static void *allocSpace(
  void *pBuf,          /* Where return pointer will be stored */
  int nByte,           /* Number of bytes to allocate */
  byte **ppFrom,         /* IN/OUT: Allocate from *ppFrom */
  byte *pEnd,            /* Pointer to 1 byte past the end of *ppFrom buffer */
  int *pnByte          /* If allocation cannot be made, increment *pnByte */
){
  assert( EIGHT_BYTE_ALIGNMENT(*ppFrom) );
  if( pBuf ) return pBuf;
  nByte = ROUND(nByte, 8);
  if( &(*ppFrom)[nByte] <= pEnd ){
    pBuf = (void*)*ppFrom;
    *ppFrom += nByte;
  }else{
    *pnByte += nByte;
  }
  return pBuf;
}

//	Rewind the VDBE back to the beginning in preparation for running it.
func (p *Vdbe) Rewind() {
	assert( p != nil )
	assert( p.magic == VDBE_MAGIC_INIT )

	//	There should be at least one opcode.
	assert( len(p.Program) > 0 )

	//	Set the magic to VDBE_MAGIC_RUN sooner rather than later.
	p.magic = VDBE_MAGIC_RUN
	p.pc = -1
	p.rc = SQLITE_OK
	p.errorAction = OE_Abort
	p.magic = VDBE_MAGIC_RUN
	p.nChange = 0
	p.cacheCtr = 1
	p.minWriteFileFormat = 255
	p.iStatement = 0
	p.nFkConstraint = 0
#ifdef VDBE_PROFILE
	for _, op := range p.Program {
		op.cnt = 0
		op.cycles = 0
	}
#endif
}

/*
** Prepare a virtual machine for execution for the first time after
** creating the virtual machine.  This involves things such
** as allocating stack space and initializing the program counter.
** After the VDBE has be prepped, it can be executed by one or more
** calls to sqlite3VdbeExec().  
**
** This function may be called exact once on a each virtual machine.
** After this routine is called the VM has been "packaged" and is ready
** to run.  After this routine is called, futher calls to 
** sqlite3VdbeAddOp() functions are prohibited.  This routine disconnects
** the Vdbe from the Parse object that helped generate it so that the
** the Vdbe becomes an independent entity and the Parse object can be
** destroyed.
**
** Use the Vdbe::Rewind() procedure to restore a virtual machine back
** to its initial state after it has been run.
*/
 void sqlite3VdbeMakeReady(
  Vdbe *p,                       /* The VDBE */
  Parse *pParse                  /* Parsing context */
){
  sqlite3 *db;                   /* The database connection */
  int nVar;                      /* Number of parameters */
  int nMem;                      /* Number of VM memory registers */
  int nCursor;                   /* Number of cursors required */
  int nArg;                      /* Number of arguments in subprograms */
  int nOnce;                     /* Number of OP_Once instructions */
  int n;                         /* Loop counter */
  byte *zCsr;                      /* Memory available for allocation */
  byte *zEnd;                      /* First byte past allocated memory */
  int nByte;                     /* How much extra memory is needed */

  assert( p != nil )
  assert( len(p.Program) > 0 )
  assert( pParse != nil )
  assert( p.magic == VDBE_MAGIC_INIT )
  db = p.db
  assert( !db.mallocFailed )
  nVar = pParse.nVar
  nMem = pParse.nMem
  nCursor = pParse.nTab
  nArg = pParse.nMaxArg
  nOnce = pParse.nOnce
  if nOnce == 0 {
	  nOnce = 1			//	Ensure at least one byte in p.aOnceFlag[]
  }
  
  /* For each cursor required, also allocate a memory cell. Memory
  ** cells (nMem+1-nCursor)..nMem, inclusive, will never be used by
  ** the vdbe program. Instead they are used to allocate space for
  ** VdbeCursor/btree.Cursor structures. The blob of memory associated with 
  ** cursor 0 is stored in memory cell nMem. Memory cell (nMem-1)
  ** stores the blob of memory associated with cursor 1, etc.
  **
  ** See also: allocateCursor().
  */
  nMem += nCursor;

  /* Allocate space for memory registers, SQL variables, VDBE cursors and 
  ** an array to marshal SQL function arguments in.
  */
  zCsr = (byte*)(&p.Program[len(p.Program)])					//	Memory avaliable for allocation
  zEnd = (byte*)(&p.Program[cap(p.Program)])					//	First byte past end of zCsr[]

  nArg = p.resolveP2Values(nArg)
  p.usesStmtJournal = (byte)(pParse.isMultiWrite && pParse.mayAbort);
  if( pParse.explain && nMem<10 ){
    nMem = 10;
  }
  memset(zCsr, 0, zEnd-zCsr);
  zCsr += (zCsr - (byte*)0)&7;
  assert( EIGHT_BYTE_ALIGNMENT(zCsr) );
  p.expired = 0;

  /* Memory for registers, parameters, cursor, etc, is allocated in two
  ** passes.  On the first pass, we try to reuse unused space at the 
  ** end of the opcode array.  If we are unable to satisfy all memory
  ** requirements by reusing the opcode array tail, then the second
  ** pass will fill in the rest using a fresh allocation.  
  **
  ** This two-pass approach that reuses as much memory as possible from
  ** the leftover space at the end of the opcode array can significantly
  ** reduce the amount of memory held by a prepared statement.
  */
  do {
    nByte = 0;
    p.aMem = allocSpace(p.aMem, nMem*sizeof(Mem), &zCsr, zEnd, &nByte);
    p.aVar = allocSpace(p.aVar, nVar*sizeof(Mem), &zCsr, zEnd, &nByte);
    p.apArg = allocSpace(p.apArg, nArg*sizeof(Mem*), &zCsr, zEnd, &nByte);
    p.azVar = allocSpace(p.azVar, nVar*sizeof(char*), &zCsr, zEnd, &nByte);
    p.apCsr = allocSpace(p.apCsr, nCursor*sizeof(VdbeCursor*),
                          &zCsr, zEnd, &nByte);
    p.aOnceFlag = allocSpace(p.aOnceFlag, nOnce, &zCsr, zEnd, &nByte);
    if( nByte ){
      p.pFree = sqlite3DbMallocZero(db, nByte);
    }
    zCsr = p.pFree;
    zEnd = &zCsr[nByte];
  }while( nByte && !db.mallocFailed );

  p.nCursor = (uint16)nCursor;
  p.nOnceFlag = nOnce;
  if( p.aVar ){
    p.nVar = (ynVar)nVar;
    for(n=0; n<nVar; n++){
      p.aVar[n].flags = MEM_Null;
      p.aVar[n].db = db;
    }
  }
  if( p.azVar ){
    p.nzVar = pParse.nzVar;
    memcpy(p.azVar, pParse.azVar, p.nzVar*sizeof(p.azVar[0]));
    memset(pParse.azVar, 0, pParse.nzVar*sizeof(pParse.azVar[0]));
  }
  if( p.aMem ){
    p.aMem--;                      /* aMem[] goes from 1..nMem */
    p.nMem = nMem;                 /*       not from 0..nMem-1 */
    for(n=1; n<=nMem; n++){
      p.aMem[n].flags = MEM_Invalid;
      p.aMem[n].db = db;
    }
  }
  p.explain = pParse.explain;
  p.Rewind()
}

//	Close a VDBE cursor and release all the resources that cursor happens to hold.
func (p *Vdbe) FreeCursor(pCx *VdbeCursor) {
	if pCx != nil {
		sqlite3VdbeSorterClose(p.db, pCx)
		switch {
		case pCx.pBt != nil:
			sqlite3BtreeClose(pCx.pBt)
		case pCx.pCursor != nil:
			sqlite3BtreeCloseCursor(pCx.pCursor)
		}
		if pCx.pVtabCursor != nil {
			p.inVtabMethod = true
			pCx.Callbacks.xClose(pCx.pVtabCursor)
			p.inVtabMethod = false
	    }
    	
  }
}

//	Copy the values stored in the VdbeFrame structure to its Vdbe. This is used, for example, when a trigger sub-program is halted to restore control to the main program.
func (pFrame *VdbeFrame) Restore() int {
	v := pFrame.v
	v.aOnceFlag = pFrame.aOnceFlag
	v.nOnceFlag = pFrame.nOnceFlag
	v.Program = pFrame.Program
	v.aMem = pFrame.aMem
	v.nMem = pFrame.nMem
	v.apCsr = pFrame.apCsr
	v.nCursor = pFrame.nCursor
	v.db.lastRowid = pFrame.lastRowid
	v.nChange = pFrame.nChange
	return pFrame.pc;
}

//	Close all cursors.
//	Also release any dynamic memory held by the VM in the Vdbe.aMem memory cell array. This is necessary as the memory cell array may contain pointers to VdbeFrame objects, which may in turn contain pointers to open cursors.
func (p *Vdbe) closeAllCursors() {
	if p.pFrame != nil {
		pFrame	*VdbeFrame
		for pFrame = p.pFrame; pFrame.pParent != nil; pFrame = pFrame.pParent {}
		pFrame.Restore()
	}
	p.pFrame = nil
	p.nFrame = 0

	if p.apCsr {
		for i := 0; i < p.nCursor; i++ {
			if cursor := p.apCsr[i]; cursor != nil {
				p.FreeCursor(cursor)
				p.apCsr[i] = nil
			}
		}
	}
	if p.aMem != nil {
		&p.aMem[1] = nil
	}
	for p.pDelFrame != nil {
		pDel := p.pDelFrame
		p.pDelFrame = pDel.pParent
		pDel.Delete()
	}
}

/*
** Clean up the VM after execution.
**
** This routine will automatically close any cursors, lists, and/or
** sorters that were left open.  It also deletes the values of
** variables in the aVar[] array.
*/
static void Cleanup(Vdbe *p){
  sqlite3 *db = p.db;

  p.zErrMsg = nil
  p.pResultSet = 0;
}

/*
** Set the number of result columns that will be returned by this SQL
** statement. This is now set at compile time, rather than during
** execution of the vdbe program so that sqlite3_column_count() can
** be called on an SQL statement before sqlite3_step().
*/
 void sqlite3VdbeSetNumCols(Vdbe *p, int nResColumn){
  Mem *pColName;
  int n;
  sqlite3 *db = p.db;

  p.ColumnsName = nil
  n = nResColumn*COLNAME_N;
  p.nResColumn = (uint16)nResColumn;
  p.ColumnsName = pColName = (Mem*)sqlite3DbMallocZero(db, sizeof(Mem)*n );
  if( p.ColumnsName==0 ) return;
  while( n-- > 0 ){
    pColName.flags = MEM_Null;
    pColName.db = p.db;
    pColName++;
  }
}

/*
** Set the name of the idx'th column to be returned by the SQL statement.
** Name must be a pointer to a nul terminated string.
**
** This call must be made after a call to sqlite3VdbeSetNumCols().
**
** The final parameter, xDel, must be one of SQLITE_DYNAMIC, SQLITE_STATIC
** or SQLITE_TRANSIENT. If it is SQLITE_DYNAMIC, then the buffer pointed
** to by Name will be freed when the vdbe is destroyed.
*/
 int sqlite3VdbeSetColName(
  Vdbe *p,                         /* Vdbe being configured */
  int idx,                         /* Index of column Name applies to */
  int var,                         /* One of the COLNAME_* constants */
  const char *Name,               /* Pointer to buffer containing name */
  void (*xDel)(void*)              /* Memory management strategy for Name */
){
  int rc;
  Mem *pColName;
  assert( idx<p.nResColumn );
  assert( var<COLNAME_N );
  if( p.db.mallocFailed ){
    assert( !Name || xDel!=SQLITE_DYNAMIC );
    return SQLITE_NOMEM;
  }
  assert( p.ColumnsName!=0 );
  pColName = &(p.ColumnsName[idx+var*p.nResColumn]);
  rc = pColName.SetStr(Name, SQLITE_UTF8, xDel)
  assert( rc!=0 || !Name || (pColName.flags&MEM_Term)!=0 );
  return rc;
}

//	A read or write transaction may or may not be active on database handle db. If a transaction is active, commit it. If there is a write-transaction spanning more than one database file, this routine takes care of the master journal trickery.
func (db *sqlite3) vdbeCommit(p *Vdbe) (rc int) {
	//	Before doing anything else, call the xSync() callback for any virtual module tables written in this transaction. This has to be done before determining whether a master journal file is required, as an xSync() callback may add an attached database to the transaction.
	rc = sqlite3VtabSync(db, &p.zErrMsg)

	//	This loop determines (a) if the commit hook should be invoked and (b) how many database files have open write transactions, not including the temp database. (b) is important because if more than one database file has an open write transaction, a master journal file is required for an atomic commit.
	nTrans := 0						//	Number of databases with an active write-transaction
	needXcommit := false
	for i, database := range db.Databases {
		if rc != SQLITE_OK {
			break
		}
		pBt := database.pBt
		if pBt.IsInTrans() {
			needXcommit = true
			if i != 1 {
				nTrans++
			}
			rc = sqlite3PagerExclusiveLock(pBt.Pager())
		}
	}

	if rc != SQLITE_OK {
		return rc
	}

	//	If there are any write-transactions at all, invoke the commit hook
	if needXcommit && db.xCommitCallback {
		if rc = db.xCommitCallback(db.pCommitArg); rc != SQLITE_OK {
			return SQLITE_CONSTRAINT
		}
	}

	//	The simple case - no more than one database file (not counting the TEMP database) has a transaction active. There is no need for the master-journal.
	//	If the return value of sqlite3BtreeGetFilename() is a zero length string, it means the main database is :memory: or a temp file. In that case we do not support atomic multi-file commits, so use the simple case then too.
	if len(sqlite3BtreeGetFilename(db.Databases[0].pBt)) == 0 || nTrans <= 1 {
		for _, database := range db.Databases {
			if rc != SQLITE_OK {
				break
			}
			if database.pBt != nil {
				rc = sqlite3BtreeCommitPhaseOne(pBt, 0)
			}
		}

		//	Do the commit only if all databases successfully complete phase 1. 
		//	If one of the BtreeCommitPhaseOne() calls fails, this indicates an IO error while deleting or truncating a journal file. It is unlikely, but could happen. In this case abandon processing and return the error.
		for _, database := range db.Databases {
			if rc != SQLITE_OK {
				break
			}
			if database.pBt != nil {
				rc = sqlite3BtreeCommitPhaseTwo(pBt, 0)
			}
		}
		if rc == SQLITE_OK {
			db.VtabCommit()
		}
	} else {
		//	The complex case - There is a multi-file write-transaction active.
		//	This requires a master journal file to ensure the transaction is committed atomicly.
		//	Select a master journal file name
		zMaster := fmt.Sprintf("%v-mjXXXXXX9XXz", sqlite3BtreeGetFilename(db.Databases[0].pBt))
		pVfs := db.pVfs
		for res, retryCount := 0, 0; ; {
			iRandom		uint32
			if retryCount != 0 {
				if retryCount > 100 {
					sqlite3_log(SQLITE_FULL, "MJ delete: %s", zMaster)
					sqlite3OsDelete(pVfs, zMaster, 0);
					break
				} else if retryCount == 1 {
					sqlite3_log(SQLITE_FULL, "MJ collide: %s", zMaster)
				}
			}
			retryCount++
			rand.Read(&iRandom)
			//	The antipenultimate character of the master journal name must be "9" to avoid name collisions when using 8+3 filenames.
			zMaster = fmt.Sprintf("%v-mj%06X9%02X", zMaster, (iRandom >> 8) & 0xffffff, iRandom & 0xff)
			rc = sqlite3OsAccess(pVfs, zMaster, SQLITE_ACCESS_EXISTS, &res)
			if rc != SQLITE_OK || res == 0 {
				break
			}
		}

		pMaster		*sqlite3_file *pMaster
		if rc == SQLITE_OK {
			//	Open the master journal.
			rc = sqlite3OsOpenMalloc(pVfs, zMaster, &pMaster,  SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_EXCLUSIVE | SQLITE_OPEN_MASTER_JOURNAL, 0)
		}
		if rc != SQLITE_OK {
			return rc
		}

		//	Write the name of each database file in the transaction into the new master journal file. If an error occurs at this point close and delete the master journal file. All the individual journal files still have 'null' as the master journal pointer, so they will roll back independently if a failure occurs.
		needSync := false
		offset := int64(0)
		for _, database := range db.Databases {
			pBt := database.pBt
			if pBt.IsInTrans() {
				zFile := sqlite3BtreeGetJournalname(pBt)
				if zFile == "" {
					continue					//	Ignore TEMP and :memory: databases
				}
				if !needSync && !sqlite3BtreeSyncDisabled(pBt) {
					needSync = true
				}
				rc = sqlite3OsWrite(pMaster, zFile, sqlite3Strlen30(zFile) + 1, offset)
				offset += sqlite3Strlen30(zFile) + 1
				if rc != SQLITE_OK {
					sqlite3OsCloseFree(pMaster)
					sqlite3OsDelete(pVfs, zMaster, 0)
					return rc
				}
			}
		}

		//	Sync the master journal file. If the IOCAP_SEQUENTIAL device flag is set this is not required.
		if needSync && (sqlite3OsDeviceCharacteristics(pMaster) & SQLITE_IOCAP_SEQUENTIAL == 0) {
			if rc = sqlite3OsSync(pMaster, SQLITE_SYNC_NORMAL); rc != SQLITE_OK {
				sqlite3OsCloseFree(pMaster)
				sqlite3OsDelete(pVfs, zMaster, 0)
				return rc
			}
		}

		//	Sync all the db files involved in the transaction. The same call sets the master journal pointer in each individual journal. If an error occurs here, do not delete the master journal file.
		//	If the error occurs during the first call to sqlite3BtreeCommitPhaseOne(), then there is a chance that the master journal file will be orphaned. But we cannot delete it, in case the master journal file name was written into the journal file before the failure occurred.
		for _, database := range db.Databases {
			if database.pBt {
				if rc = sqlite3BtreeCommitPhaseOne(database.pBt, zMaster); rc != SQLITE_OK {
					break
				}
			}
		}
		sqlite3OsCloseFree(pMaster)
		assert( rc != SQLITE_BUSY )
		if rc != SQLITE_OK {
			return rc
		}

		//	Delete the master journal file. This commits the transaction. After doing this the directory is synced again before any individual transaction files are deleted.
		rc = sqlite3OsDelete(pVfs, zMaster, 1)
		if rc != SQLITE_OK {
			return rc
		}

		//	All files and directories have already been synced, so the following calls to sqlite3BtreeCommitPhaseTwo() are only closing files and deleting or truncating journals. If something goes wrong while this is happening we don't really care. The integrity of the transaction is already guaranteed, but some stray 'cold' journals may be lying around. Returning an error code won't help matters.
		disable_simulated_io_errors()
		for _, database := range db.Databases {
			if database.pBt != nil {
				sqlite3BtreeCommitPhaseTwo(database.pBt, 1)
			}
		}
		enable_simulated_io_errors()
		db.VtabCommit()
	}
	return rc
}

//	If the Vdbe passed as the first argument opened a statement-transaction, close it now. Argument eOp must be either SAVEPOINT_ROLLBACK or SAVEPOINT_RELEASE. If it is SAVEPOINT_ROLLBACK, then the statement transaction is rolled back. If eOp is SAVEPOINT_RELEASE, then the statement transaction is commtted.
//	If an IO error occurs, an SQLITE_IOERR_XXX error code is returned. Otherwise SQLITE_OK.
func (p *Vdbe) CloseStatement(eOp int) (rc int) {
	db := p.db
	rc = SQLITE_OK

	//	If p.iStatement is greater than zero, then this Vdbe opened a statement transaction that should be closed here. The only exception is that an IO error may have occured, causing an emergency rollback. In this case (db.nStatement==0), and there is nothing to do.
	if db.nStatement != 0 && p.iStatement != 0 {
		iSavepoint := p.iStatement - 1

		assert( eOp == SAVEPOINT_ROLLBACK || eOp == SAVEPOINT_RELEASE)
		assert( db.nStatement > 0 )
		assert( p.iStatement == db.nStatement + db.nSavepoint) )
		for _, database := range db.Databases {
			rc2 := SQLITE_OK
			if database.pBt != nil {
				if eOp == SAVEPOINT_ROLLBACK {
					rc2 = database.pBt.BtreeSavepoint(SAVEPOINT_ROLLBACK, iSavepoint)
				}
				if rc2 == SQLITE_OK {
					rc2 = database.pBt.BtreeSavepoint(SAVEPOINT_RELEASE, iSavepoint)
				}
				if rc == SQLITE_OK {
					rc = rc2
				}
			}
		}
		db.nStatement--
		p.iStatement = 0

		if rc == SQLITE_OK {
			if eOp == SAVEPOINT_ROLLBACK {
				rc = sqlite3VtabSavepoint(db, SAVEPOINT_ROLLBACK, iSavepoint)
			}
			if rc == SQLITE_OK {
				rc = sqlite3VtabSavepoint(db, SAVEPOINT_RELEASE, iSavepoint)
			}
		}

		//	If the statement transaction is being rolled back, also restore the database handles deferred constraint counter to the value it had when the statement transaction was opened.
		if eOp == SAVEPOINT_ROLLBACK {
			db.nDeferredCons = p.nStmtDefCons
		}
	}
	return
}

//	This function is called when a transaction opened by the database handle associated with the VM passed as an argument is about to be committed. If there are outstanding deferred foreign key constraint violations, return SQLITE_ERROR. Otherwise, SQLITE_OK.
//	If there are outstanding FK violations and this function returns SQLITE_ERROR, set the result of the VM to SQLITE_CONSTRAINT and write an error message to it. Then return SQLITE_ERROR.
func (p *Vdbe) CheckFk(deferred bool) (rc int) {
	db := p.db
	if (deferred && db.nDeferredCons > 0) || (!deferred && p.nFkConstraint > 0) {
		p.rc = SQLITE_CONSTRAINT
		p.errorAction = OE_Abort
		p.zErrMsg = "foreign key constraint failed"
		rc = SQLITE_ERROR
	}
	return
}

//	This routine is called the when a VDBE tries to halt. If the VDBE has made changes and is in autocommit mode, then commit those changes. If a rollback is needed, then do the rollback.
//	This routine is the only way to move the state of a VM from SQLITE_MAGIC_RUN to SQLITE_MAGIC_HALT. It is harmless to call this on a VM that is in the SQLITE_MAGIC_HALT state.
//	Return an error code. If the commit could not complete because of lock contention, return SQLITE_BUSY. If SQLITE_BUSY is returned, it means the close did not happen and needs to be repeated.
func (p *Vdbe) Halt() (rc int) {
	db := p.db

	//	This function contains the logic that determines if a statement or transaction will be committed or rolled back as a result of the execution of this virtual machine. 
	//	If any of the following errors occur:
	//			SQLITE_NOMEM
	//			SQLITE_IOERR
	//			SQLITE_FULL
	//			SQLITE_INTERRUPT
	//	Then the internal cache might have been left in an inconsistent state. We need to rollback the statement transaction, if there is one, or the complete transaction if there is no statement transaction.
	if p.db.mallocFailed {
		p.rc = SQLITE_NOMEM
	}
	if p.aOnceFlag {
		memset(p.aOnceFlag, 0, p.nOnceFlag)
	}
	p.closeAllCursors()
	if p.magic != VDBE_MAGIC_RUN {
		return SQLITE_OK
	}

	//	No commit or rollback needed if the program never started
	if p.pc >= 0 {
		eStatementOp := 0

		//	Lock all btrees used by the statement
		p.Enter()

		//	Check for one of the special errors
		mrc := p.rc & 0xff					//	Primary error code from p.rc
		isSpecialError := mrc == SQLITE_NOMEM || mrc == SQLITE_IOERR || mrc == SQLITE_INTERRUPT || mrc == SQLITE_FULL
		if isSpecialError {
			//	If the query was read-only and the error code is SQLITE_INTERRUPT, no rollback is necessary. Otherwise, at least a savepoint transaction must be rolled back to restore the database to a consistent state.
			//	Even if the statement is read-only, it is important to perform a statement or transaction rollback operation. If the error occured while writing to the journal, sub-journal or database file as part of an effort to free up cache space (see function pagerStress() in pager.c), the rollback is required to restore the pager to a consistent state.
			if !p.readOnly || mrc != SQLITE_INTERRUPT {
				if (mrc == SQLITE_NOMEM || mrc == SQLITE_FULL) && p.usesStmtJournal {
					eStatementOp = SAVEPOINT_ROLLBACK
				} else {
					//	We are forced to roll back the active transaction. Before doing so, abort any other statements this handle currently has active.
					db.RollbackAll(SQLITE_ABORT_ROLLBACK)
					db.CloseSavepoints()
					db.autoCommit = 1
				}
			}
		}

		//	Check for immediate foreign key violations.
		if p.rc == SQLITE_OK {
			p.CheckFk(0)
		}
  
		//	If the auto-commit flag is set and this is the only active writer VM, then we do either a commit or rollback of the current transaction. 
		//	Note: This block also runs if one of the special errors handled above has occurred.
		if !(db.nVTrans > 0 && db.aVTrans == 0) && db.autoCommit && db.writeVdbeCnt == (p.readOnly == 0) {
			if p.rc == SQLITE_OK || (p.errorAction == OE_Fail && !isSpecialError) {
				if rc = p.CheckFk(1); rc != SQLITE_OK {
					if p.readOnly {
						p.Leave()
						return SQLITE_ERROR
					}
					rc = SQLITE_CONSTRAINT
				} else {
					//	The auto-commit flag is true, the vdbe program was successful or hit an 'OR FAIL' constraint and there are no deferred foreign key constraints to hold up the transaction. This means a commit is required.
					rc = db.vdbeCommit(p)
				}
				switch {
				case rc == SQLITE_BUSY && p.readOnly:
					p.Leave()
					return SQLITE_BUSY
				case rc != SQLITE_OK:
					p.rc = rc
					db.RollbackAll(SQLITE_OK)
				default:
					db.nDeferredCons = 0
					db.CommitInternalChanges()
				}
			} else {
				db.RollbackAll(SQLITE_OK)
			}
			db.nStatement = 0
		} else if eStatementOp == 0 {
			switch {
			case p.rc == SQLITE_OK || p.errorAction == OE_Fail:
				eStatementOp = SAVEPOINT_RELEASE
			case p.errorAction == OE_Abort:
				eStatementOp = SAVEPOINT_ROLLBACK
			default:
				db.RollbackAll(SQLITE_ABORT_ROLLBACK)
				db.CloseSavepoints()
				db.autoCommit = 1
			}
		}
  
		//	If eStatementOp is non-zero, then a statement transaction needs to be committed or rolled back. Call Vdbe::CloseStatement() to do so. If this operation returns an error, and the current statement error code is SQLITE_OK or SQLITE_CONSTRAINT, then promote the current statement error code.
		if eStatementOp {
			if rc = p.CloseStatement(eStatementOp); rc != SQLITE_OK {
				if p.rc == SQLITE_OK || p.rc == SQLITE_CONSTRAINT {
					p.rc = rc
					p.zErrMsg = nil
				}
				db.RollbackAll(SQLITE_ABORT_ROLLBACK)
				db.CloseSavepoints()
				db.autoCommit = 1
			}
		}

		//	If this was an INSERT, UPDATE or DELETE and no statement transaction has been rolled back, update the database connection change-counter. 
		if p.changeCntOn {
			if eStatementOp != SAVEPOINT_ROLLBACK {
				db.VdbeSetChanges(p.nChange)
			} else {
				db.VdbeSetChanges(0)
			}
			p.nChange = 0
		}

		//	Release the locks
		p.Leave()
	}

	//	We have successfully halted and closed the VM. Record this fact.
	if p.pc >= 0 {
		db.activeVdbeCnt--
		if !p.readOnly {
			db.writeVdbeCnt--
		}
		assert( db.activeVdbeCnt >= db.writeVdbeCnt )
	}
	p.magic = VDBE_MAGIC_HALT
	if p.db.mallocFailed {
		p.rc = SQLITE_NOMEM
	}

	//	If the auto-commit flag is set to true, then any locks that were held by connection db have now been released. Call sqlite3::ConnectionUnlocked() to invoke any required unlock-notify callbacks.
	if db.autoCommit {
		db.ConnectionUnlocked()
	}
	assert( db.activeVdbeCnt > 0 || db.autoCommit == 0 || db.nStatement == 0 )
	if p.rc == SQLITE_BUSY {
		rc = SQLITE_BUSY
	} else {
		rc = SQLITE_OK
	}
	return
}

//	Each VDBE holds the result of the most recent sqlite3_step() call in p.rc. This routine sets that result back to SQLITE_OK.
func (p *Vdbe) ResetStepResult() {
	p.rc = SQLITE_OK
}

/*
** Copy the error code and error message belonging to the VDBE passed
** as the first argument to its database handle (so that they will be 
** returned by calls to sqlite3_errcode() and sqlite3_errmsg()).
**
** This function does not clear the VDBE error code or message, just
** copies them to the database handle.
*/
 int sqlite3VdbeTransferError(Vdbe *p){
  sqlite3 *db = p.db;
  int rc = p.rc;
  if( p.zErrMsg ){
    byte mallocFailed = db.mallocFailed;
    sqlite3ValueSetStr(db.pErr, -1, p.zErrMsg, SQLITE_UTF8, SQLITE_TRANSIENT);
    db.mallocFailed = mallocFailed;
    db.errCode = rc;
  }else{
    db.Error(rc, "");
  }
  return rc;
}

//	Clean up a VDBE after execution but do not delete the VDBE just yet. Write any error messages into *pzErrMsg. Return the result code.
//	After this routine is run, the VDBE should be ready to be executed again.
//	To look at it another way, this routine resets the state of the virtual machine from VDBE_MAGIC_RUN or VDBE_MAGIC_HALT back to VDBE_MAGIC_INIT.
func (p *Vdbe) Reset() int {
	db := p.db

	//	If the VM did not run to completion or if it encountered an error, then it might not have been halted properly. So halt it now.
	p.Halt()

	//	If the VDBE has be run even partially, then transfer the error code and error message from the VDBE into the main database structure. But if the VDBE has just been set to run but has not actually executed any instructions yet, leave the main database error information unchanged.
	if p.pc >= 0 {
		sqlite3VdbeTransferError(p)
		p.zErrMsg = nil
		if p.runOnlyOnce {
			p.expired = 1
		}
	} else if p.rc != SQLITE_OK && p.expired {
		//	The expired flag was set on the VDBE before the first call to sqlite3_step(). For consistency (since sqlite3_step() was called), set the database error in this case as well.
		db.Error(p.rc, "")
		sqlite3ValueSetStr(db.pErr, -1, p.zErrMsg, SQLITE_UTF8, SQLITE_TRANSIENT)
		p.zErrMsg = nil
	}

	//	Reclaim all memory used by the VDBE
	Cleanup(p)

	//	Save profiling information from this VDBE run.
#ifdef VDBE_PROFILE
	if out := fopen("vdbe_profile.out", "a"); out != nil {
		int i;
		fmt.Fprint(out, "---- ")
		for _, op := range p.Program {
			fmt.Fprintf(out, "%02x", op.opcode)
		}
		fprintf(out, "\n")
		for i, op := range p.Program {
			if op.cnt > 0 {
				fmt.Ffprintf(out, "%6d %10lld %8lld ", op.cnt, op.cycles, op.cycles / op.cnt)
			} else {
				fmt.Ffprintf(out, "%6d %10lld %8lld ", op.cnt, op.cycles, 0)
			}
			sqlite3VdbePrintOp(out, i, &p.Program[i])
		}
		fclose(out)
	}
#endif
	p.magic = VDBE_MAGIC_INIT
	return p.rc & db.errMask
}
 
/*
** Clean up and delete a VDBE after execution.  Return an integer which is
** the result code.  Write any error message text into *pzErrMsg.
*/
 int sqlite3VdbeFinalize(Vdbe *p){
  int rc = SQLITE_OK;
  if( p.magic==VDBE_MAGIC_RUN || p.magic==VDBE_MAGIC_HALT ){
    rc = p.Reset();
    assert( (rc & p.db.errMask)==rc );
  }
  sqlite3VdbeDelete(p);
  return rc;
}

/*
** Call the destructor for each auxdata entry in pVdbeFunc for which
** the corresponding bit in mask is clear.  Auxdata entries beyond 31
** are always destroyed.  To destroy all auxdata entries, call this
** routine with mask==0.
*/
 void sqlite3VdbeDeleteAuxData(VdbeFunc *pVdbeFunc, int mask){
  int i;
  for(i=0; i<pVdbeFunc.nAux; i++){
    struct AuxData *pAux = &pVdbeFunc.apAux[i];
    if( (i>31 || !(mask&(((uint32)1)<<i))) && pAux.Parameter ){
      if( pAux.xDelete ){
        pAux.xDelete(pAux.Parameter);
      }
      pAux.Parameter = 0;
    }
  }
}

//	Free all memory associated with the Vdbe passed as the second argument.
//	The difference between this function and sqlite3VdbeDelete() is that VdbeDelete() also unlinks the Vdbe from the list of VMs associated with the database connection.
void sqlite3VdbeDeleteObject(sqlite3 *db, Vdbe *p){
	assert( p.db == nil || p.db == db )
	p.aVar = nil
	p.ColumnsName = nil
	for i, routine := range p.Routines {
		db.FreeOpArray(routine)
		p.Routines[i] = nil
	}
	p.Routines = nil
	for i := p.nzVar - 1; i >= 0; i-- {
		p.azVar[i] = nil
	}
	p.azVar[i] = nil
	db.FreeOpArray(p.Program)
	p.aLabel = nil
	p.ColumnsName = nil
	p.zSql = nil
	p.pFree = nil
	p = nil
}

/*
** Delete an entire VDBE.
*/
 void sqlite3VdbeDelete(Vdbe *p){
  sqlite3 *db;

  if( p==0 ) return;
  db = p.db;
  if( p.pPrev ){
    p.pPrev.Next = p.Next;
  }else{
    assert( db.pVdbe==p );
    db.pVdbe = p.Next;
  }
  if( p.Next ){
    p.Next.pPrev = p.pPrev;
  }
  p.magic = VDBE_MAGIC_DEAD;
  p.db = 0;
  sqlite3VdbeDeleteObject(db, p);
}

/*
** Make sure the cursor p is ready to read or write the row to which it
** was last positioned.  Return an error code if an OOM fault or I/O error
** prevents us from positioning the cursor to its correct position.
**
** If a MoveTo operation is pending on the given cursor, then do that
** MoveTo now.  If no move is pending, check to see if the row has been
** deleted out from under the cursor and if it has, mark the row as
** a NULL row.
**
** If the cursor is already pointing to the correct row and that row has
** not been deleted out from under the cursor, then this routine is a no-op.
*/
 int sqlite3VdbeCursorMoveto(VdbeCursor *p){
  if( p.deferredMoveto ){
    int res, rc;
    assert( p.isTable );
    rc = sqlite3BtreeMovetoUnpacked(p.pCursor, 0, p.movetoTarget, 0, &res);
    if( rc ) return rc;
    p.lastRowid = p.movetoTarget;
    if( res!=0 ) return SQLITE_CORRUPT_BKPT;
    p.rowidIsValid = 1;
    p.deferredMoveto = 0;
    p.cacheStatus = CACHE_STALE;
  }else if p.pCursor {
    int hasMoved;
    int rc = sqlite3BtreeCursorHasMoved(p.pCursor, &hasMoved);
    if( rc ) return rc;
    if( hasMoved ){
      p.cacheStatus = CACHE_STALE;
      p.nullRow = 1;
    }
  }
  return SQLITE_OK;
}

//	The following functions:
//			VdbeSerialType()
//			VdbeSerialTypeLen()
//			VdbeSerialPut()
//			VdbeSerialGet()
//	encapsulate the code that serializes values for storage in SQLite data and index records. Each serialized value consists of a 'serial-type' and a blob of data. The serial type is an 8-byte unsigned integer, stored as a varint.
//	In an SQLite index record, the serial type is stored directly before the blob of data that it corresponds to. In a table record, all serial types are stored at the start of the record, and the blobs of data at the end. Hence these functions allow the caller to handle the serial-type and data blob seperately.
//	The following table describes the various storage classes for data:
//			 serial type        bytes of data      type
//			--------------     ---------------    ---------------
//				0                     0            NULL
//				1                     1            signed integer
//				2                     2            signed integer
//				3                     3            signed integer
//				4                     4            signed integer
//				5                     6            signed integer
//				6                     8            signed integer
//				7                     8            IEEE float
//				8                     0            Integer constant 0
//				9                     0            Integer constant 1
//				10, 11                             reserved for expansion
//				N >= 12 and even  (N - 12) / 2     BLOB
//				N >= 13 and odd   (N - 13) / 2     text
//	The 8 and 9 types were added in 3.3.0, file format 4. Prior versions of SQLite will not understand those serial types.

var MAX_6BYTE int64 = (0x00008000 << 32) - 1

//	Return the serial-type for the value stored in pMem.
func (pMem *Mem) VdbeSerialType(file_format int) (count uint32) {
	if flags := pMem.flags; flags & MEM_Null == 0 {
		switch pMem.Value.(type) {
		case int64:
			//	Figure out whether to use 1, 2, 4, 6 or 8 bytes.
			i := pMem.Integer()
			if file_format >= 4 && (i & 1) == i {
				return 8 + uint32(i)
			}

			var u	uint64
			if i < 0 {
				if i < -MAX_6BYTE {
					return 6
				}
				//	Previous test prevents:  u = -(-9223372036854775808)
				u = -i
			} else {
				u = i
			}
			switch {
			case u <= 127:
				count = 1
			case u <= 32767:
				count = 2
			case u <= 8388607:
				count = 3
			case u <= 2147483647:
				count = 4
			case u <= MAX_6BYTE:
				count = 5
			default:
				count = 6
			}
		case flags & MEM_Real != 0:
			count = 7
		default:
			assert( pMem.db.mallocFailed || flags & (MEM_Str | MEM_Blob) )
			n := pMem.ByteLen()
			assert( n >= 0 )
			if flags & MEM_Str == 0 {
				count = ((n * 2) + 12 + ((flags & MEM_Str) != 0))
			} else {
				count = ((n * 2) + 12 + ((flags & MEM_Str) != 0))
			}
		}
	}
	return
}

//	Return the length of the data corresponding to the supplied serial-type.
func VdbeSerialTypeLen(serial_type uint32) uint32 {
	if serial_type >= 12 {
		return (serial_type - 12) / 2
	}
	aSize := []byte{ 0, 1, 2, 3, 4, 6, 8, 8, 0, 0, 0, 0 }
	return aSize[serial_type]
}

//	Write the serialized data blob for the value stored in pMem into buf. It is assumed that the caller has allocated sufficient space. Return the number of bytes written.
//	nBuf is the amount of space left in buf[]. nBuf must always be large enough to hold the entire field. Except, if the field is a blob with a zero-filled tail, then buf[] might be just the right size to hold everything except for the zero-filled tail. If buf[] is only big enough to hold the non-zero prefix, then only write that prefix into buf[]. But if buf[] is large enough to hold both the prefix and the tail then write the prefix and set the tail to all zeros.
//	Return the number of bytes actually written into buf[]. The number of bytes in the zero-filled tail is included in the return value only if those bytes were zeroed in buf[].
func (pMem *Mem) VdbeSerialPut(buf []byte, file_format int) (count uint32) {
	switch serial_type := pMem.VdbeSerialType(file_format); {
	case serial_type > 0 && serial_type <= 7:	//	Integer and Real
		var v		uint64
		if serial_type == 7 {
			v = math.Float64bits(pMem.r)
		} else {
			v = pMem.Integer()
		}
		count = VdbeSerialTypeLen(serial_type)
		assert( count <= uint32(len(buf)) )
		for i := l; i != 0; i-- {
			buf[i] = byte(v & 0xFF)
			v >>= 8
		}
	case serial_type >= 12:						//	String or blob
		if v, ok := pMem.Value.(Zeroes); ok {
			assert( pMem.n + int(v) == int(VdbeSerialTypeLen(serial_type)) )
		} else {
			assert( pMem.n == int(VdbeSerialTypeLen(serial_type)) )
		}
		assert( pMem.n <= len(buf) )
		count = pMem.n
		copy(buf, pMem.z[:l])
		if v, ok := pMem.Value.(Zeroes); ok {
			l += int(v)
			assert( len(buf) >= 0 )
			if l > uint32(len(buf)) {
				l = uint32(len(buf))
			}
			tail := buf[pMem.n:]
			for i, _ := range tail {
				tail[i] = 0
			}
		}
	}
	return										//	NULL or constants 0 or 1 have a count == 0
}

//	Deserialize the data blob pointed to by buf as serial type serial_type and store the result in pMem. Return the number of bytes read.
func (pMem *Mem) VdbeSerialGet(buf []byte, serial_type uint32) (count uint32) {
	switch serial_type {
	case 10:					//	Reserved for future use
	case 11:					//	Reserved for future use
	case 0:						//	NULL
		pMem.flags = MEM_Null
	case 1:						//	1-byte signed integer
		pMem.Store(int64(buf[0]))
		count = 1
    case 2:						//	2-byte signed integer
		pMem.Store(int64(buf[0]) << 8 | int64(buf[1]))
		count = 2
    case 3:						//	3-byte signed integer
		pMem.Store(int64(buf[0]) << 16 | int64(buf[1]) << 8 | int64(buf[2]))
		count = 3
    case 4:						//	4-byte signed integer
		pMem.Store(int64(buf[0]) << 24 | int64(buf[1]) << 16 | int64(buf[2]) << 8 | int64(buf[3]))
		count = 4
    case 5:						//	6-byte signed integer
		x := int64(buf[0]) << 8 | int64(buf[1])
		y := int64(buf[2]) << 24 | int64(buf[3]) << 16 | int64(buf[4]) << 8 | int64(buf[5])
		x = x << 32 | y
		pMem.Store(x)
		count = 6
	case 6:						//	8-byte signed integer
		x := int64(buf[0]) << 24 | int64(buf[1]) << 16 | int64(buf[2]) << 8 | int64(buf[3])
		y := int64(buf[4]) << 24 | int64(buf[5]) << 16 | int64(buf[6]) << 8 | int64(buf[7])
		x = x << 32 | y
		pMem.Store(x)
		count = 8
	case 7:						//	IEEE floating point
		x := int64(buf[0]) << 24 | int64(buf[1]) << 16 | int64(buf[2]) << 8 | int64(buf[3])
		y := int64(buf[4]) << 24 | int64(buf[5]) << 16 | int64(buf[6]) << 8 | int64(buf[7])
		pMem.r = math.Float64frombits(x << 32 | y)
		pMem.flags = math.IsNaN(pMem.r) ? MEM_Null : MEM_Real
		count = 8
    case 8:		fallthrough		//	Integer 0
    case 9:						//	Integer 1
		pMem.Store(int64(serial_type - 8))
	default:
		count = (serial_type - 12) / 2
		pMem.z = string(byte[:count])
		pMem.n = count
		pMem.xDel = nil
		if serial_type & 0x01 != 0 {
			pMem.flags = MEM_Str | MEM_Ephem
		} else {
			pMem.flags = MEM_Blob | MEM_Ephem
		}
	}
	return
}

/*
** This routine is used to allocate sufficient space for an UnpackedRecord
** structure large enough to be used with sqlite3VdbeRecordUnpack() if
** the first argument is a pointer to KeyInfo structure pKeyInfo.
**
** The space is either allocated using sqlite3DbMallocRaw() or from within
** the unaligned buffer passed via the second and third arguments (presumably
** stack space). If the former, then *ppFree is set to a pointer. Or, if the 
** allocation comes from the pSpace/szSpace buffer, *ppFree is set to NULL
** before returning.
**
** If an OOM error occurs, NULL is returned.
*/
 UnpackedRecord *sqlite3VdbeAllocUnpackedRecord(
  KeyInfo *pKeyInfo,              /* Description of the record */
  char *pSpace,                   /* Unaligned space available */
  int szSpace,                    /* Size of pSpace[] in bytes */
  char **ppFree                   /* OUT: Caller should free this pointer */
){
  UnpackedRecord *p;              /* Unpacked record to return */
  int nOff;                       /* Increment pSpace by nOff to align it */
  int nByte;                      /* Number of bytes required for *p */

  /* We want to shift the pointer pSpace up such that it is 8-byte aligned.
  ** Thus, we need to calculate a value, nOff, between 0 and 7, to shift 
  ** it by.  If pSpace is already 8-byte aligned, nOff should be zero.
  */
  nOff = (8 - (SQLITE_PTR_TO_INT(pSpace) & 7)) & 7;
  nByte = ROUND(sizeof(UnpackedRecord), 8) + sizeof(Mem)*(pKeyInfo.nField+1);
  if( nByte>szSpace+nOff ){
    p = (UnpackedRecord *)sqlite3DbMallocRaw(pKeyInfo.db, nByte);
    *ppFree = (char *)p;
    if( !p ) return 0;
  }else{
    p = (UnpackedRecord*)&pSpace[nOff];
    *ppFree = 0;
  }

  p.aMem = (Mem*)&((char*)p)[ROUND(sizeof(UnpackedRecord), 8)];
  p.pKeyInfo = pKeyInfo;
  p.nField = pKeyInfo.nField + 1;
  return p;
}

/*
** Given the nKey-byte encoding of a record in pKey[], populate the 
** UnpackedRecord structure indicated by the fourth argument with the
** contents of the decoded record.
*/ 
 void sqlite3VdbeRecordUnpack(
  KeyInfo *pKeyInfo,     /* Information about the record format */
  int nKey,              /* Size of the binary record */
  const void *pKey,      /* The binary record */
  UnpackedRecord *p      /* Populate this structure before returning. */
){
  const unsigned char *aKey = (const unsigned char *)pKey;
  int d; 
  uint32 idx;                        /* Offset in aKey[] to read from */
  uint16 u;                          /* Unsigned loop counter */
  uint32 szHdr;
  Mem *pMem = p.aMem;

  p.flags = 0;
  assert( EIGHT_BYTE_ALIGNMENT(pMem) );
  idx = getVarint32(aKey, szHdr);
  d = szHdr;
  u = 0;
  while( idx<szHdr && u<p.nField && d<=nKey ){
    uint32 serial_type;

    idx += getVarint32(&aKey[idx], serial_type);
    pMem.enc = pKeyInfo.enc;
    pMem.db = pKeyInfo.db;
    /* pMem.flags = 0; // VdbeSerialGet() will set this for us */
    pMem.zMalloc = 0;
    d += pMem.VdbeSerialGet(&aKey[d:], serial_type)
    pMem++;
    u++;
  }
  assert( u<=pKeyInfo.nField + 1 );
  p.nField = u;
}

/*
** This function compares the two table rows or index records
** specified by {nKey1, pKey1} and pPKey2.  It returns a negative, zero
** or positive integer if key1 is less than, equal to or 
** greater than key2.  The {nKey1, pKey1} key must be a blob
** created by th OP_MakeRecord opcode of the VDBE.  The pPKey2
** key must be a parsed key such as obtained from
** sqlite3VdbeParseRecord.
**
** Key1 and Key2 do not have to contain the same number of fields.
** The key with fewer fields is usually compares less than the 
** longer key.  However if the UNPACKED_INCRKEY flags in pPKey2 is set
** and the common prefixes are equal, then key1 is less than key2.
** Or if the UNPACKED_MATCH_PREFIX flag is set and the prefixes are
** equal, then the keys are considered to be equal and
** the parts beyond the common prefix are ignored.
*/
 int sqlite3VdbeRecordCompare(
  int nKey1, const void *pKey1, /* Left key */
  UnpackedRecord *pPKey2        /* Right key */
){
  int d1;            /* Offset into aKey[] of next data element */
  uint32 idx1;          /* Offset into aKey[] of next header element */
  uint32 szHdr1;        /* Number of bytes in header */
  int i = 0;
  int nField;
  int rc = 0;
  const unsigned char *aKey1 = (const unsigned char *)pKey1;
  KeyInfo *pKeyInfo;
  Mem mem1;

  pKeyInfo = pPKey2.pKeyInfo;
  mem1.enc = pKeyInfo.enc;
  mem1.db = pKeyInfo.db;
  /* mem1.flags = 0;  // Will be initialized by VdbeSerialGet() */
  VVA_ONLY( mem1.zMalloc = 0; ) /* Only needed by assert() statements */

  idx1 = getVarint32(aKey1, szHdr1);
  d1 = szHdr1;
  nField = pKeyInfo.nField;
  while( idx1<szHdr1 && i<pPKey2.nField ){
    uint32 serial_type1;

    /* Read the serial types for the next element in each key. */
    idx1 += getVarint32( aKey1+idx1, serial_type1 );
    if( d1>=nKey1 && VdbeSerialTypeLen(serial_type1)>0 ) break;

    /* Extract the values to be compared.
    */
    d1 += mem1.VdbeSerialGet(&aKey1[d1:], serial_type1)

    /* Do the comparison
    */
    rc = sqlite3MemCompare(&mem1, &pPKey2.aMem[i],
                           i<nField ? pKeyInfo.Collations[i] : 0);
    if( rc!=0 ){
      assert( mem1.zMalloc==0 );  /* See comment below */

      /* Invert the result if we are using DESC sort order. */
      if( pKeyInfo.aSortOrder && i<nField && pKeyInfo.aSortOrder[i] ){
        rc = -rc;
      }
    
		//	If the PREFIX_SEARCH flag is set and all fields except the final rowid field were equal, then clear the PREFIX_SEARCH flag and set pPKey2.rowid to the value of the rowid field in (pKey1, nKey1). This is used by the OP_IsUnique opcode.
	  	if pPKey2.flags & UNPACKED_PREFIX_SEARCH && i == pPKey2.nField - 1 {
			assert( idx1 == szHdr1 && rc )
			pPKey2.flags &= ~UNPACKED_PREFIX_SEARCH
			pPKey2.rowid = mem1.Integer()
		}
		return rc
    }
    i++;
  }

  /* No memory allocation is ever used on mem1.  Prove this using
  ** the following assert().  If the assert() fails, it indicates a
  ** memory leak and a need to call mem1.Release().
  */
  assert( mem1.zMalloc==0 );

  /* rc==0 here means that one of the keys ran out of fields and
  ** all the fields up to that point were equal. If the UNPACKED_INCRKEY
  ** flag is set, then break the tie by treating key2 as larger.
  ** If the UPACKED_PREFIX_MATCH flag is set, then keys with common prefixes
  ** are considered to be equal.  Otherwise, the longer key is the 
  ** larger.  As it happens, the pPKey2 will always be the longer
  ** if there is a difference.
  */
  assert( rc==0 );
  if( pPKey2.flags & UNPACKED_INCRKEY ){
    rc = -1;
  }else if( pPKey2.flags & UNPACKED_PREFIX_MATCH ){
    /* Leave rc==0 */
  }else if( idx1<szHdr1 ){
    rc = 1;
  }
  return rc;
}
 

/*
** pCur points at an index entry created using the OP_MakeRecord opcode.
** Read the rowid (the last field in the record) and store it in *rowid.
** Return SQLITE_OK if everything works, or an error code otherwise.
**
** pCur might be pointing to text obtained from a corrupt database file.
** So the content cannot be trusted.  Do appropriate checks on the content.
*/
 int sqlite3VdbeIdxRowid(sqlite3 *db, btree.Cursor *pCur, int64 *rowid){
  int64 nCellKey = 0;
  int rc;
  uint32 szHdr;        /* Size of the header */
  uint32 typeRowid;    /* Serial type of the rowid */
  uint32 lenRowid;     /* Size of the rowid */
  Mem m, v;

  /* Get the size of the index entry.  Only indices entries of less
  ** than 2GiB are support - anything large must be database corruption.
  ** Any corruption is detected in sqlite3BtreeParseCellPtr(), though, so
  ** this code can safely assume that nCellKey is 32-bits  
  */
  assert( sqlite3BtreeCursorIsValid(pCur) );
  VVA_ONLY(rc =) sqlite3BtreeKeySize(pCur, &nCellKey);
  assert( rc==SQLITE_OK );     /* pCur is always valid so KeySize cannot fail */
  assert( (nCellKey & SQLITE_MAX_U32)==(uint64)nCellKey );

  /* Read in the complete content of the index entry */
  memset(&m, 0, sizeof(m));
  rc = sqlite3VdbeMemFromBtree(pCur, 0, (int)nCellKey, 1, &m);
  if( rc ){
    return rc;
  }

  /* The index entry must begin with a header size */
  (void)getVarint32((byte*)m.z, szHdr);
  if szHdr<3 || (int)szHdr > m.n {
    goto idx_rowid_corruption;
  }

  /* The last field of the index should be an integer - the ROWID.
  ** Verify that the last entry really is an integer. */
  (void)getVarint32((byte*)&m.z[szHdr-1], typeRowid);
  if typeRowid < 1 || typeRowid > 9 || typeRowid == 7 {
    goto idx_rowid_corruption;
  }
  lenRowid = VdbeSerialTypeLen(typeRowid)
  if (uint32) m.n < szHdr+lenRowid {
    goto idx_rowid_corruption;
  }

  /* Fetch the integer off the end of the index record */
  v.VdbeSerialGet(([]byte)(m.z)[m.n - lenRowid:], typeRowid)
  *rowid = v.Integer()
  m.Release()
  return SQLITE_OK;

  /* Jump here if database corruption is detected after m has been
  ** allocated.  Free the m object and return SQLITE_CORRUPT. */
idx_rowid_corruption:
  m.Release()
  return SQLITE_CORRUPT_BKPT;
}

/*
** Compare the key of the index entry that cursor pC is pointing to against
** the key string in pUnpacked.  Write into *pRes a number
** that is negative, zero, or positive if pC is less than, equal to,
** or greater than pUnpacked.  Return SQLITE_OK on success.
**
** pUnpacked is either created without a rowid or is truncated so that it
** omits the rowid at the end.  The rowid at the end of the index entry
** is ignored as well.  Hence, this routine only compares the prefixes 
** of the keys prior to the final rowid, not the entire key.
*/
 int sqlite3VdbeIdxKeyCompare(
  VdbeCursor *pC,             /* The cursor to compare against */
  UnpackedRecord *pUnpacked,  /* Unpacked version of key to compare against */
  int *res                    /* Write the comparison result here */
){
  int64 nCellKey = 0;
  int rc;
  btree.Cursor *pCur = pC.pCursor;
  Mem m;

  assert( sqlite3BtreeCursorIsValid(pCur) );
  VVA_ONLY(rc =) sqlite3BtreeKeySize(pCur, &nCellKey);
  assert( rc==SQLITE_OK );    /* pCur is always valid so KeySize cannot fail */
  /* nCellKey will always be between 0 and 0xffffffff because of the way
  ** that CellInfo::ParsePtr() and sqlite3GetVarint32() are implemented */
  if( nCellKey<=0 || nCellKey>0x7fffffff ){
    *res = 0;
    return SQLITE_CORRUPT_BKPT;
  }
  memset(&m, 0, sizeof(m));
  rc = sqlite3VdbeMemFromBtree(pC.pCursor, 0, (int)nCellKey, 1, &m);
  if( rc ){
    return rc;
  }
  assert( pUnpacked.flags & UNPACKED_PREFIX_MATCH );
  *res = sqlite3VdbeRecordCompare(m.n, m.z, pUnpacked);
  m.Release()
  return SQLITE_OK;
}

//	This routine sets the value to be returned by subsequent calls to sqlite3_changes() on the database handle 'db'. 
func (db *sqlite3) VdbeSetChanges(nChange int) {
	db.nChange = nChange
	db.nTotalChange += nChange
}

/*
** Set a flag in the vdbe to update the change counter when it is finalised
** or reset.
*/
 void sqlite3VdbeCountChanges(Vdbe *v){
  v.changeCntOn = 1;
}

//	Mark every prepared statement associated with a database connection as expired.
//	An expired statement means that recompilation of the statement is recommend. Statements expire when things happen that make their programs obsolete. Removing user-defined functions or collating sequences, or changing an authorization function are the types of things that make prepared statements obsolete.
func (db *sqlite3) ExpirePreparedStatements() {
	for p := db.pVdbe; p != nil; p = p.Next {
		p.expired = 1
	}
}

//	Return the database associated with the Vdbe.
func (v *Vdbe) DB() *sqlite3 {
	return v.db
}

/*
** Return a pointer to an sqlite3_value structure containing the value bound
** parameter iVar of VM v. Except, if the value is an SQL NULL, return 
** 0 instead. Unless it is NULL, apply affinity aff (one of the SQLITE_AFF_*
** constants) to the value before returning it.
**
** The returned value must be freed by the caller using Mem::Free().
*/
 sqlite3_value *sqlite3VdbeGetValue(Vdbe *v, int iVar, byte aff){
  assert( iVar>0 );
  if( v ){
    Mem *pMem = &v.aVar[iVar-1];
    if( 0==(pMem.flags & MEM_Null) ){
      sqlite3_value *pRet = v.db.NewValue()
      if( pRet ){
        sqlite3VdbeMemCopy((Mem *)pRet, pMem);
        sqlite3ValueApplyAffinity(pRet, aff, SQLITE_UTF8);
        (Mem *)(pRet).StoreType()
      }
      return pRet;
    }
  }
  return 0;
}

/*
** Configure SQL variable iVar so that binding a new value to it signals
** to sqlite3_reoptimize() that re-preparing the statement may result
** in a better query plan.
*/
 void sqlite3VdbeSetVarmask(Vdbe *v, int iVar){
  assert( iVar>0 );
  if( iVar>32 ){
    v.expmask = 0xffffffff;
  }else{
    v.expmask |= ((uint32)1 << (iVar-1));
  }
}
