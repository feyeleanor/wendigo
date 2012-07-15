import "crypto/rand"

/* The code in this file implements execution method of the 
** Virtual Database Engine (VDBE).  A separate file ("vdbeaux.c")
** handles housekeeping details such as creating and deleting
** VDBE instances.  This file is solely interested in executing
** the VDBE program.
**
** In the external interface, an "sqlite3_stmt*" is an opaque pointer
** to a VDBE.
**
** The SQL parser generates a program which is then executed by
** the VDBE to do the work of the SQL statement.  VDBE programs are 
** similar in form to assembly language.  The program consists of
** a linear sequence of operations.  Each operation has an opcode 
** and 5 operands.  Operands P1, P2, and P3 are integers.  Operand P4 
** is a null-terminated string.  Operand P5 is an unsigned character.
** Few opcodes use all 5 operands.
**
** Computation results are stored on a set of registers numbered beginning
** with 1 and going up to Vdbe.nMem.  Each register can store
** either an integer, a null-terminated string, a floating point
** number, or the SQL "NULL" value.  An implicit conversion from one
** type to the other occurs as necessary.
** 
** Most of the code in this file is taken up by the sqlite3VdbeExec()
** function which does the work of interpreting a VDBE program.
** But other routines are also provided to help in building up
** a program instruction by instruction.
**
** Various scripts scan this source file in order to generate HTML
** documentation, headers files, or other derived files.  The formatting
** of the code in this file is, therefore, important.  See other comments
** in this file for details.  If in doubt, do not deviate from existing
** commenting and indentation practices when changing or adding code.
*/

/*
** Invoke this macro on memory cells just prior to changing the
** value of the cell.  This macro verifies that shallow copies are
** not misused.
*/
# define memAboutToChange(P,M)

/*
** Convert the given register into a string if it isn't one
** already. Return non-zero if a malloc() fails.
*/
#define Stringify(P, enc)		if ((P).flags&(MEM_Str|MEM_Blob))==0 && P.Stringify(enc) != SQLITE_OK { goto no_mem }

/*
** An ephemeral string value (signified by the MEM_Ephem flag) contains
** a pointer to a dynamically allocated string where some other entity
** is responsible for deallocating that string.  Because the register
** does not control the string, it might be deleted without the register
** knowing it.
**
** This routine converts an ephemeral string into a dynamically allocated
** string that the register itself controls.  In other words, it
** converts an MEM_Ephem string into an MEM_Dyn string.
*/
#define Deephemeralize(P) \
   if( ((P).flags&MEM_Ephem)!=0 \
       && sqlite3VdbeMemMakeWriteable(P) ){ goto no_mem;}

/* Return true if the cursor was opened using the OP_OpenSorter opcode. */
# define isSorter(x) ((x).pSorter!=0)

//	Argument pMem points at a register that will be passed to a user-defined function or returned to the user as the result of a query. This routine sets the pMem.Type variable used by the sqlite3_value_*() routines.
func (pMem *Mem) StoreType() {
	int flags = pMem.flags
	switch pMem.Value.(type) {
	case nil:
		pMem.Type = SQLITE_NULL
	case int64:
		pMem.Type = SQLITE_INTEGER
	case flags & MEM_Real:
		pMem.Type = SQLITE_FLOAT
	case flags & MEM_Str:
		pMem.Type = SQLITE_TEXT
	default:
		pMem.Type = SQLITE_BLOB
	}
}

/*
** Allocate VdbeCursor number iCur.  Return a pointer to it.  Return NULL
** if we run out of memory.
*/
static VdbeCursor *allocateCursor(
  Vdbe *p,              /* The virtual machine */
  int iCur,             /* Index of the new VdbeCursor */
  int nField,           /* Number of fields in the table or index */
  int iDb,              /* Database the cursor belongs to, or -1 */
  int isBtreeCursor     /* True for B-Tree.  False for pseudo-table or vtab */
){
  /* Find the memory cell that will be used to store the blob of memory
  ** required for this VdbeCursor structure. It is convenient to use a 
  ** vdbe memory cell to manage the memory allocation required for a
  ** VdbeCursor structure for the following reasons:
  **
  **   * Sometimes cursor numbers are used for a couple of different
  **     purposes in a vdbe program. The different uses might require
  **     different sized allocations. Memory cells provide growable
  **     allocations.
  **
  **   * When using ENABLE_MEMORY_MANAGEMENT, memory cell buffers can
  **     be freed lazily via the ReleaseMemory() API. This
  **     minimizes the number of malloc calls made by the system.
  **
  ** Memory cells for cursors are allocated at the top of the address
  ** space. Memory cell (p.nMem) corresponds to cursor 0. Space for
  ** cursor 1 is managed by memory cell (p.nMem-1), etc.
  */
  Mem *pMem = &p.aMem[p.nMem-iCur];

  int nByte;
  VdbeCursor *pCx = 0;
  nByte = 
      ROUND(sizeof(VdbeCursor), 8) + 
      (isBtreeCursor?sqlite3BtreeCursorSize():0) + 
      2*nField*sizeof(uint32);

  assert( iCur<p.nCursor );
  if( p.apCsr[iCur] ){
    p.FreeCursor(p.apCsr[iCur])
    p.apCsr[iCur] = nil
  }
  if( SQLITE_OK==sqlite3VdbeMemGrow(pMem, nByte, 0) ){
    p.apCsr[iCur] = pCx = (VdbeCursor*)pMem.z;
    memset(pCx, 0, sizeof(VdbeCursor));
    pCx.iDb = iDb;
    pCx.nField = nField;
    if( nField ){
      pCx.aType = (uint32 *)&pMem.z[ROUND(sizeof(VdbeCursor), 8)];
    }
    if( isBtreeCursor ){
      pCx.pCursor = (btree.Cursor*)
          &pMem.z[ROUND(sizeof(VdbeCursor), 8)+2*nField*sizeof(uint32)];
      sqlite3BtreeCursorZero(pCx.pCursor);
    }
  }
  return pCx;
}

//	Try to convert a value into a numeric representation if we can do so without loss of information. In other words, if the string looks like a number, convert it into a number. If it does not look like a number, leave it alone.
func (pRec *Mem) applyNumericAffinity() {
	if pRec.flags & MEM_Real == 0 {
		if pRec.flags & MEM_Str != 0 {
			if rValue, err := strconv.ParseFloat(pRec.z, 64); err == nil {
				if v, e := strconv.ParseInt(pRec.z, 0, 64); e == nil {
					pRec.Store(v)
				} else {
					pRec.r = rValue
					pRec.flags |= MEM_Real
				}
			}
		}
	}
}

//	Processing is determine by the affinity parameter:
//		SQLITE_AFF_INTEGER:
//		SQLITE_AFF_REAL:
//		SQLITE_AFF_NUMERIC:
//			Try to convert pRec to an integer representation or a floating-point representation if an integer representation is not possible. Note that the integer representation is always preferred, even if the affinity is REAL, because an integer representation is more space efficient on disk.
//		SQLITE_AFF_TEXT:
//			Convert pRec to a text representation.
//		SQLITE_AFF_NONE:
//			No-op. pRec is unchanged.
static void applyAffinity(
  Mem *pRec,          /* The value to apply affinity to */
  char affinity,      /* The affinity to be applied */
  byte enc              /* Use this text encoding */
){
	if affinity == SQLITE_AFF_TEXT {
		//	Only attempt the conversion to TEXT if there is an integer or real representation (blob and NULL do not get converted) but no string representation.
		switch pRec.Value.(type) {
		case int64, pRec.flags & MEM_Real == 1:
			pRec.Stringify(enc)
		}
		pRec.flags &= ~MEM_Real
	} else if affinity != SQLITE_AFF_NONE {
		assert( affinity == SQLITE_AFF_INTEGER || affinity == SQLITE_AFF_REAL || affinity == SQLITE_AFF_NUMERIC )
		pRec.applyNumericAffinity()
		if pRec.flags & MEM_Real != 0 {
			pRec.IntegerAffinity()
		}
	}
}

//	Try to convert the type of a function argument or a result column into a numeric representation. Use either INTEGER or REAL whichever is appropriate. But only do the conversion if it is possible without loss of information and return the revised type of the argument.
func (pVal *sqlite3_value) NumericType() int {
	pMem := (Mem*)(pVal)
	if pMem.Type == SQLITE_TEXT {
		pMem.applyNumericAffinity()
		pMem.StoreType()
	}
	return pMem.Type
}

/*
** Exported version of applyAffinity(). This one works on sqlite3_value*, 
** not the internal Mem* type.
*/
 void sqlite3ValueApplyAffinity(
  sqlite3_value *pVal, 
  byte affinity, 
  byte enc
){
  applyAffinity((Mem *)pVal, affinity, enc);
}


#  define REGISTER_TRACE(R,M)

#ifdef VDBE_PROFILE

/* 
** hwtime.h contains inline assembler code for implementing 
** high-performance timing routines.
*/
/************** Include hwtime.h in the middle of vdbe.c *********************/
/************** Begin file hwtime.h ******************************************/
/* This file contains inline asm code for retrieving "high-performance"
** counters for x86 class CPUs.
*/
#ifndef _HWTIME_H_
#define _HWTIME_H_

/*
** The following routine only works on pentium-class (or newer) processors.
** It uses the RDTSC opcode to read the cycle count value out of the
** processor and returns that value.  This can be used for high-res
** profiling.
*/
  #error Need implementation of sqlite3Hwtime() for your platform.

  /*
  ** To compile without implementing sqlite3Hwtime() for your platform,
  ** you can remove the above #error and use the following
  ** stub function.  You will lose timing support for many
  ** of the debugging and testing utilities, but it should at
  ** least compile and run.
  */
   uint64 sqlite3Hwtime(void){ return ((uint64)0); }

#endif /* !defined(_HWTIME_H_) */

/************** End of hwtime.h **********************************************/
/************** Continuing where we left off in vdbe.c ***********************/

#endif

/*
** The CHECK_FOR_INTERRUPT macro defined here looks to see if the
** sqlite3_interrupt() routine has been called.  If it has been, then
** processing of the VDBE program is interrupted.
**
** This macro added to every instruction that does a jump in order to
** implement a loop.  This test used to be on every single instruction,
** but that meant we more testing than we needed.  By only testing the
** flag on jump instructions, we get a (small) speed improvement.
*/
#define CHECK_FOR_INTERRUPT \
   if( db.u1.isInterrupted ) goto abort_due_to_interrupt;

/*
** Transfer error message text from an sqlite3_vtab.zErrMsg (text stored
** in memory obtained from sqlite3_malloc) into a Vdbe.zErrMsg (text stored
** in memory obtained from sqlite3DbMalloc).
*/
static void importVtabErrMsg(Vdbe *p, sqlite3_vtab *pVtab){
  sqlite3 *db = p.db;
  p.zErrMsg = sqlite3DbStrDup(db, pVtab.zErrMsg);
  pVtab.zErrMsg = nil
}


/*
** Execute as much of a VDBE program as we can then return.
**
** sqlite3VdbeMakeReady() must be called before this routine in order to
** close the program with a final OP_Halt and to set up the callbacks
** and the error message pointer.
**
** Whenever a row or result data is available, this routine will either
** invoke the result callback (if there is one) or return with
** SQLITE_ROW.
**
** If an attempt is made to open a locked database, then this routine
** will either invoke the busy callback (if there is one) or it will
** return SQLITE_BUSY.
**
** If an error occurs, an error message is written to memory obtained
** from sqlite3_malloc() and p.zErrMsg is made to point to that memory.
** The error code is stored in p.rc and this routine returns SQLITE_ERROR.
**
** If the callback ever returns non-zero, then the program exits
** immediately.  There will be no error message but the p.rc field is
** set to SQLITE_ABORT and this routine will return SQLITE_ERROR.
**
** A memory allocation error causes p.rc to be set to SQLITE_NOMEM and this
** routine to return SQLITE_ERROR.
**
** Other fatal errors return SQLITE_ERROR.
**
** After this routine has finished, sqlite3VdbeFinalize() should be
** used to clean up the mess that was left behind.
*/
 int sqlite3VdbeExec(
  Vdbe *p                    /* The VDBE */
){
	pc :=0							//	The program counter
	program := p.Program
  Op *pOp;                   /* Current operation */
  int rc = SQLITE_OK;        /* Value to return */
  sqlite3 *db = p.db;       /* The database */
  byte resetSchemaOnFault = 0; /* Reset schema after an error if positive */

	encoding := db.Encoding()     /* The database encoding */

#ifndef SQLITE_OMIT_PROGRESS_CALLBACK
  int checkProgress;         /* True if progress callbacks are enabled */
  int nProgressOps = 0;      /* Opcodes executed since progress callback. */
#endif
  Mem *aMem = p.aMem;       /* Copy of p.aMem */
  Mem *pIn1 = 0;             /* 1st input operand */
  Mem *pIn2 = 0;             /* 2nd input operand */
  Mem *pIn3 = 0;             /* 3rd input operand */
  Mem *pOut = 0;             /* Output operand */
  int iCompare = 0;          /* Result of last OP_Compare operation */
  int *aPermute = 0;         /* Permutation of columns for OP_Compare */
  int64 lastRowid = db.lastRowid;  /* Saved value of the last insert ROWID */
#ifdef VDBE_PROFILE
  uint64 start;                 /* CPU clock count at start of opcode */
  int origPc;                /* Program counter at start of opcode */
#endif
  /********************************************************************
  ** Automatically generated code
  **
  ** The following union is automatically generated by the
  ** vdbe-compress.tcl script.  The purpose of this union is to
  ** reduce the amount of stack space required by this function.
  ** See comments in the vdbe-compress.tcl script for details.
  */
  union vdbeExecUnion {
    struct OP_Yield_stack_vars {
      int pcDest;
    } aa;
    struct OP_Null_stack_vars {
      int cnt;
    } ab;
    struct OP_Variable_stack_vars {
      Mem *pVar;       /* Value being transferred */
    } ac;
    struct OP_Move_stack_vars {
      char *zMalloc;   /* Holding variable for allocated memory */
      int n;           /* Number of registers left to copy */
      int p1;          /* Register to copy from */
      int p2;          /* Register to copy to */
    } ad;
    struct OP_ResultRow_stack_vars {
      Mem *pMem;
      int i;
    } ae;
    struct OP_Concat_stack_vars {
      int64 nByte;
    } af;
    struct OP_Remainder_stack_vars {
      int flags;      /* Combined MEM_* flags from both inputs */
      int64 iA;         /* Integer value of left operand */
      int64 iB;         /* Integer value of right operand */
      double rA;      /* Real value of left operand */
      double rB;      /* Real value of right operand */
    } ag;
    struct OP_Function_stack_vars {
      int i;
      Mem *pArg;
      sqlite3_context ctx;
      sqlite3_value **apVal;
      int n;
    } ah;
    struct OP_ShiftRight_stack_vars {
      int64 iA;
      uint64 uA;
      int64 iB;
      byte op;
    } ai;
    struct OP_Ge_stack_vars {
      int res;            /* Result of the comparison of pIn1 against pIn3 */
      char affinity;      /* Affinity to use for comparison */
      uint16 flags1;         /* Copy of initial value of pIn1.flags */
      uint16 flags3;         /* Copy of initial value of pIn3.flags */
    } aj;
    struct OP_Compare_stack_vars {
      int n;
      int i;
      int p1;
      int p2;
      const KeyInfo *pKeyInfo;
      int idx;
      CollSeq *pColl;    /* Collating sequence to use on this term */
      int bRev;          /* True for DESCENDING sort order */
    } ak;
    struct OP_Or_stack_vars {
      int v1;    /* Left operand:  0==FALSE, 1==TRUE, 2==UNKNOWN or NULL */
      int v2;    /* Right operand: 0==FALSE, 1==TRUE, 2==UNKNOWN or NULL */
    } al;
    struct OP_IfNot_stack_vars {
      int c;
    } am;
    struct OP_Column_stack_vars {
      uint32 payloadSize;   /* Number of bytes in the record */
      int64 payloadSize64; /* Number of bytes in the record */
      int p1;            /* P1 value of the opcode */
      int p2;            /* column number to retrieve */
      VdbeCursor *pC;    /* The VDBE cursor */
      char *zRec;        /* Pointer to complete record-data */
      btree.Cursor *pCrsr;   /* The BTree cursor */
      uint32 *aType;        /* aType[i] holds the numeric type of the i-th column */
      uint32 *aOffset;      /* aOffset[i] is offset to start of data for i-th column */
      int nField;        /* number of fields in the record */
      int len;           /* The length of the serialized data for the column */
      int i;             /* Loop counter */
      char *zData;       /* Part of the record being decoded */
      Mem *pDest;        /* Where to write the extracted value */
      Mem sMem;          /* For storing the record being decoded */
      byte *zIdx;          /* Index into header */
      byte *zEndHdr;       /* Pointer to first byte after the header */
      uint32 offset;        /* Offset into the data */
      uint32 szField;       /* Number of bytes in the content of a field */
      int szHdr;         /* Size of the header size field at start of record */
      int avail;         /* Number of bytes of available data */
      uint32 t;             /* A type code from the record header */
      Mem *pReg;         /* PseudoTable input register */
    } an;
    struct OP_Affinity_stack_vars {
      const char *zAffinity;   /* The affinity to be applied */
      char cAff;               /* A single character of affinity */
    } ao;
    struct OP_MakeRecord_stack_vars {
      byte *zNewRecord;        /* A buffer to hold the data for the new record */
      Mem *pRec;             /* The new record */
      uint64 nData;             /* Number of bytes of data space */
      int nHdr;              /* Number of bytes of header space */
      int64 nByte;             /* Data space required for this record */
      int nZero;             /* Number of zero bytes at the end of the record */
      int nVarint;           /* Number of bytes in a varint */
      uint32 serial_type;       /* Type field */
      Mem *pData0;           /* First field to be combined into the record */
      Mem *pLast;            /* Last field of the record */
      int nField;            /* Number of fields in the record */
      char *zAffinity;       /* The affinity string for the record */
      int file_format;       /* File format to use for encoding */
      int i;                 /* Space used in zNewRecord[] */
      int len;               /* Length of a field */
    } ap;
    struct OP_Count_stack_vars {
      int64 nEntry;
      btree.Cursor *pCrsr;
    } aq;
    struct OP_Savepoint_stack_vars {
      int p1;                         /* Value of P1 operand */
      char *Name;                    /* Name of savepoint */
      int nName;
      Savepoint *pNew;
      Savepoint *pSavepoint;
      Savepoint *pTmp;
      int iSavepoint;
      int ii;
    } ar;
    struct OP_AutoCommit_stack_vars {
      int desiredAutoCommit;
      int iRollback;
      int turnOnAC;
    } as;
    struct OP_Transaction_stack_vars {
      Btree *pBt;
    } at;
    struct OP_ReadCookie_stack_vars {
      int iMeta;
      int iDb;
      int iCookie;
    } au;
    struct OP_SetCookie_stack_vars {
      Db *pDb;
    } av;
    struct OP_VerifyCookie_stack_vars {
      int iMeta;
      int iGen;
      Btree *pBt;
    } aw;
    struct OP_OpenWrite_stack_vars {
      int nField;
      KeyInfo *pKeyInfo;
      int p2;
      int iDb;
      int Writable;
      Btree *pX;
      VdbeCursor *pCur;
      Db *pDb;
    } ax;
    struct OP_OpenEphemeral_stack_vars {
      VdbeCursor *pCx;
    } ay;
    struct OP_SorterOpen_stack_vars {
      VdbeCursor *pCx;
    } az;
    struct OP_OpenPseudo_stack_vars {
      VdbeCursor *pCx;
    } ba;
    struct OP_SeekGt_stack_vars {
      int res;
      int oc;
      VdbeCursor *pC;
      UnpackedRecord r;
      int nField;
      int64 iKey;      /* The rowid we are to seek to */
    } bb;
    struct OP_Seek_stack_vars {
      VdbeCursor *pC;
    } bc;
    struct OP_Found_stack_vars {
      int alreadyExists;
      VdbeCursor *pC;
      int res;
      char *pFree;
      UnpackedRecord *pIdxKey;
      UnpackedRecord r;
      char aTempRec[ROUND(sizeof(UnpackedRecord), 8) + sizeof(Mem)*3 + 7];
    } bd;
    struct OP_IsUnique_stack_vars {
      uint16 ii;
      VdbeCursor *pCx;
      btree.Cursor *pCrsr;
      uint16 nField;
      Mem *aMx;
      UnpackedRecord r;                  /* B-Tree index search key */
      int64 R;                             /* Rowid stored in register P3 */
    } be;
    struct OP_NotExists_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      int res;
      uint64 iKey;
    } bf;
    struct OP_NewRowid_stack_vars {
      int64 v;                 /* The new rowid */
      VdbeCursor *pC;        /* Cursor of table to get the new rowid */
      int res;               /* Result of an sqlite3BtreeLast() */
      int cnt;               /* Counter to limit the number of searches */
      Mem *pMem;             /* Register holding largest rowid for AUTOINCREMENT */
      VdbeFrame *pFrame;     /* Root frame of VDBE */
    } bg;
    struct OP_InsertInt_stack_vars {
      Mem *pData;       /* MEM cell holding data for the record to be inserted */
      Mem *pKey;        /* MEM cell holding key  for the record */
      int64 iKey;         /* The integer ROWID or key for the record to be inserted */
      VdbeCursor *pC;   /* Cursor to table into which insert is written */
      int nZero;        /* Number of zero-bytes to append */
      int seekResult;   /* Result of prior seek or 0 if no USESEEKRESULT flag */
      const char *zDb;  /* database name - used by the update hook */
      const char *zTbl; /* Table name - used by the opdate hook */
      int op;           /* Opcode for update hook: SQLITE_UPDATE or SQLITE_INSERT */
    } bh;
    struct OP_Delete_stack_vars {
      int64 iKey;
      VdbeCursor *pC;
    } bi;
    struct OP_SorterCompare_stack_vars {
      VdbeCursor *pC;
      int res;
    } bj;
    struct OP_SorterData_stack_vars {
      VdbeCursor *pC;
    } bk;
    struct OP_RowData_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      uint32 n;
      int64 n64;
    } bl;
    struct OP_Rowid_stack_vars {
      VdbeCursor *pC;
      int64 v;
      sqlite3_vtab *pVtab;
      const sqlite3_module *pModule;
    } bm;
    struct OP_NullRow_stack_vars {
      VdbeCursor *pC;
    } bn;
    struct OP_Last_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      int res;
    } bo;
    struct OP_Rewind_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      int res;
    } bp;
    struct OP_Next_stack_vars {
      VdbeCursor *pC;
      int res;
    } bq;
    struct OP_IdxInsert_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      int nKey;
      const char *zKey;
    } br;
    struct OP_IdxDelete_stack_vars {
      VdbeCursor *pC;
      btree.Cursor *pCrsr;
      int res;
      UnpackedRecord r;
    } bs;
    struct OP_IdxRowid_stack_vars {
      btree.Cursor *pCrsr;
      VdbeCursor *pC;
      int64 rowid;
    } bt;
    struct OP_IdxGE_stack_vars {
      VdbeCursor *pC;
      int res;
      UnpackedRecord r;
    } bu;
    struct OP_Destroy_stack_vars {
      int iMoved;
      int iCnt;
      Vdbe *pVdbe;
      int iDb;
    } bv;
    struct OP_Clear_stack_vars {
      int nChange;
    } bw;
    struct OP_CreateTable_stack_vars {
      int pgno;
      int flags;
      Db *pDb;
    } bx;
    struct OP_ParseSchema_stack_vars {
      int iDb;
      const char *zMaster;
      char *zSql;
      InitData initData;
    } by;
    struct OP_IntegrityCheck_stack_vars {
      int nRoot;      /* Number of tables to check.  (Number of root pages.) */
      int *aRoot;     /* Array of rootpage numbers for tables to be checked */
      int j;          /* Loop counter */
      int nErr;       /* Number of errors reported */
      char *z;        /* Text of the error report */
      Mem *pnErr;     /* Register keeping track of errors remaining */
    } bz;
    struct OP_RowSetRead_stack_vars {
      int64 val;
    } ca;
    struct OP_RowSetTest_stack_vars {
      int iSet;
      int exists;
    } cb;
    struct OP_Program_stack_vars {
      int nMem;               /* Number of memory registers for sub-program */
      int nByte;              /* Bytes of runtime space required for sub-program */
      Mem *pRt;               /* Register to allocate runtime space */
      Mem *pMem;              /* Used to iterate through memory cells */
      Mem *pEnd;              /* Last memory cell in new array */
      VdbeFrame *pFrame;      /* New vdbe frame to execute in */
      *SubProgram;   		/* Sub-program to execute */
      void *t;                /* Token identifying trigger */
    } cc;
    struct OP_Param_stack_vars {
      VdbeFrame *pFrame;
      Mem *pIn;
    } cd;
    struct OP_MemMax_stack_vars {
      Mem *pIn1;
      VdbeFrame *pFrame;
    } ce;
    struct OP_AggStep_stack_vars {
      int n;
      int i;
      Mem *pMem;
      Mem *pRec;
      sqlite3_context ctx;
      sqlite3_value **apVal;
    } cf;
    struct OP_AggFinal_stack_vars {
      Mem *pMem;
    } cg;
    struct OP_Checkpoint_stack_vars {
      int i;                          /* Loop counter */
      int aRes[3];                    /* Results */
      Mem *pMem;                      /* Write results here */
    } ch;
    struct OP_JournalMode_stack_vars {
      Btree *pBt;                     /* Btree to change journal mode of */
      Pager *pPager;                  /* Pager associated with pBt */
      int eNew;                       /* New journal mode */
      int eOld;                       /* The old journal mode */
      const char *zFilename;          /* Name of database file for pPager */
    } ci;
    struct OP_IncrVacuum_stack_vars {
      Btree *pBt;
    } cj;
    struct OP_VBegin_stack_vars {
      VTable *pVTab;
    } ck;
    struct OP_VOpen_stack_vars {
      VdbeCursor *pCur;
      sqlite3_vtab_cursor *pVtabCursor;
      sqlite3_vtab *pVtab;
      sqlite3_module *pModule;
    } cl;
    struct OP_VFilter_stack_vars {
      int nArg;
      int iQuery;
      const sqlite3_module *pModule;
      Mem *pQuery;
      Mem *pArgc;
      sqlite3_vtab_cursor *pVtabCursor;
      sqlite3_vtab *pVtab;
      VdbeCursor *pCur;
      int res;
      int i;
      Mem **apArg;
    } cm;
    struct OP_VColumn_stack_vars {
      sqlite3_vtab *pVtab;
      const sqlite3_module *pModule;
      Mem *pDest;
      sqlite3_context sContext;
    } cn;
    struct OP_VNext_stack_vars {
      sqlite3_vtab *pVtab;
      const sqlite3_module *pModule;
      int res;
      VdbeCursor *pCur;
    } co;
    struct OP_VRename_stack_vars {
      sqlite3_vtab *pVtab;
      Mem *pName;
    } cp;
    struct OP_VUpdate_stack_vars {
      sqlite3_vtab *pVtab;
      sqlite3_module *pModule;
      int nArg;
      int i;
      sqlite_int64 rowid;
      Mem **apArg;
      Mem *pX;
    } cq;
    struct OP_Trace_stack_vars {
      char *zTrace;
      char *z;
    } cr;
  } u;
  /* End automatically generated code
  ********************************************************************/

  assert( p.magic==VDBE_MAGIC_RUN );  /* sqlite3_step() verifies this */
  p.Enter()
  if( p.rc==SQLITE_NOMEM ){
    /* This happens if a malloc() inside a call to sqlite3_column_text() failed.  */
    goto no_mem;
  }
  assert( p.rc==SQLITE_OK || p.rc==SQLITE_BUSY );
  p.rc = SQLITE_OK;
  assert( p.explain==0 );
  p.pResultSet = 0;
  db.busyHandler.nBusy = 0;
  CHECK_FOR_INTERRUPT;
  p.IOTraceSql(p)
#ifndef SQLITE_OMIT_PROGRESS_CALLBACK
  checkProgress = db.xProgress!=0;
#endif
  for pc = p.pc; rc == SQLITE_OK; pc++ {
    assert( pc >= 0 && pc < len(p.Program) )
    if( db.mallocFailed ) goto no_mem;
#ifdef VDBE_PROFILE
    origPc = pc;
    start = sqlite3Hwtime();
#endif
    pOp = program[pc]

#ifndef SQLITE_OMIT_PROGRESS_CALLBACK
    /* Call the progress callback if it is configured and the required number
    ** of VDBE ops have been executed (either since this invocation of
    ** sqlite3VdbeExec() or since last time the progress callback was called).
    ** If the progress callback returns non-zero, exit the virtual machine with
    ** a return code SQLITE_ABORT.
    */
    if( checkProgress ){
      if( db.nProgressOps==nProgressOps ){
        int prc;
        prc = db.xProgress(db.pProgressArg);
        if( prc!=0 ){
          rc = SQLITE_INTERRUPT;
          goto vdbe_error_halt;
        }
        nProgressOps = 0;
      }
      nProgressOps++;
    }
#endif

    /* On any opcode with the "out2-prerelase" tag, free any
    ** external allocations out of mem[p2] and set mem[p2] to be
    ** an undefined integer.  Opcodes will either fill in the integer
    ** value or convert mem[p2] to a different type.
    */
    assert( pOp.opflags==OPCODE_PROPERTIES[pOp.opcode] );
    if( pOp.opflags & OPFLG_OUT2_PRERELEASE ){
      assert( pOp.p2>0 );
      assert( pOp.p2<=p.nMem );
      pOut = &aMem[pOp.p2];
      memAboutToChange(p, pOut);
      VdbeMemRelease(pOut);
      pOut.flags = ;
    }

    switch( pOp.opcode ){

/*****************************************************************************
** What follows is a massive switch statement where each case implements a
** separate instruction in the virtual machine.  If we follow the usual
** indentation conventions, each case should be indented by 6 spaces.  But
** that is a lot of wasted space on the left margin.  So the code within
** the switch statement will break with convention and be flush-left. Another
** big comment (similar to this one) will mark the point in the code where
** we transition back to normal indentation.
**
** The formatting of each case is important.  The makefile for SQLite
** generates two C files "opcodes.h" and "opcodes.c" by scanning this
** file looking for lines that begin with "case OP_".  The opcodes.h files
** will be filled with #defines that give unique integer values to each
** opcode and the opcodes.c file is filled with an array of strings where
** each string is the symbolic name for the corresponding opcode.  If the
** case statement is followed by a comment of the form "/# same as ... #/"
** that comment is used to determine the particular value of the opcode.
**
** Other keywords in the comment that follows each case are used to
** construct the OPFLG_INITIALIZER value that initializes opcodeProperty[].
** Keywords include: in1, in2, in3, out2_prerelease, out2, out3.  See
** the mkopcodeh.awk script for additional information.
**
** Documentation about VDBE opcodes is generated by scanning this file
** for lines of that contain "Opcode:".  That line and all subsequent
** comment lines are used in the generation of the opcode.html documentation
** file.
**
** SUMMARY:
**
**     Formatting is important to scripts that scan this file.
**     Do not deviate from the formatting style currently in use.
**
*****************************************************************************/

/* Opcode:  Goto * P2 * * *
**
** An unconditional jump to address P2.
** The next instruction executed will be 
** the one at index P2 from the beginning of
** the program.
*/
case OP_Goto: {             /* jump */
  CHECK_FOR_INTERRUPT;
  pc = pOp.p2 - 1;
  break;
}

/* Opcode:  Gosub P1 P2 * * *
**
** Write the current address onto register P1
** and then jump to address P2.
*/
case OP_Gosub: {            /* jump */
  assert( pOp.p1>0 && pOp.p1<=p.nMem );
  pIn1 = &aMem[pOp.p1];
  assert( (pIn1.flags & MEM_Dyn)==0 );
  memAboutToChange(p, pIn1);
  pIn1.flags = ;
  pIn1.Store(pc)
  REGISTER_TRACE(pOp.p1, pIn1);
  pc = pOp.p2 - 1;
  break;
}

//	Opcode:  Return P1 * * * *
//	Jump to the next instruction after the address in register P1.
case OP_Return:				//	in1
	pIn1 = &aMem[pOp.p1]
	assert( pIn1.flags &  )
	pc = pIn1.Integer()

//	Opcode:  Yield P1 * * * *
//	Swap the program counter with the value in register P1.
case OP_Yield:				//	in1
	pIn1 = &aMem[pOp.p1]
	assert( pIn1.flags & MEM_Dyn == 0 )
	pIn1.flags = 
	u.aa.pcDest = pIn1.Integer()
	pIn1.Store(pc)
	REGISTER_TRACE(pOp.p1, pIn1)
	pc = u.aa.pcDest

//	Opcode:  HaltIfNull  P1 P2 P3 P4 *
//	Check the value in register P3. If it is NULL then Halt using parameter P1, P2, and P4 as if this were a Halt instruction. If the value in register P3 is not NULL, then this routine is a no-op.
case OP_HaltIfNull:			//	in3
	pIn3 = &aMem[pOp.p3]
	if pIn3.Value != nil {
		break
	}
	fallthrough

//	Opcode:  Halt P1 P2 * P4 *
//	Exit immediately. All open cursors, etc are closed automatically.
//	P1 is the result code returned by sqlite3_exec(), sqlite3_reset(), or sqlite3_finalize(). For a normal halt, this should be SQLITE_OK (0). For errors, it can be some other value. If P1 != 0 then P2 will determine whether or not to rollback the current transaction. Do not rollback if P2 == OE_Fail. Do the rollback if P2 == OE_Rollback. If P2 == OE_Abort, then back out all changes that have occurred during this execution of the VDBE, but do not rollback the transaction.
//	If P4 is not null then it is an error message string.
//	There is an implied "Halt 0 0 0" instruction inserted at the very end of every program. So a jump past the last instruction of the program is the same as executing Halt.
case OP_Halt: {
	if pOp.p1 == SQLITE_OK && p.pFrame {
		//	Halt the sub-program. Return control to the parent frame.
		pFrame := p.pFrame
		p.pFrame = pFrame.pParent
		p.nFrame--
		db.VdbeSetChanges(p.nChange)
		pc = pFrame.Restore()
		lastRowid = db.lastRowid
		if pOp.p2 == OE_Ignore {
			//	Instruction pc is the OP_Program that invoked the sub-program currently being halted. If the p2 instruction of this OP_Halt instruction is set to OE_Ignore, then the sub-program is throwing an IGNORE exception. In this case jump to the address specified as the p2 of the calling OP_Program.
			pc = p.Program[pc].p2 - 1
		}
		program = p.Program
		aMem = p.aMem
		break
	}

	p.rc = pOp.p1
	p.errorAction = byte(pOp.p2)
	p.pc = pc
	if pOp.p4.z {
		assert( p.rc != SQLITE_OK )
		p.zErrMsg = pOp.p4.z
		sqlite3_log(pOp.p1, "abort at %d in [%s]: %s", pc, p.zSql, pOp.p4.z)
	} else if p.rc != SQLITE_OK {
		sqlite3_log(pOp.p1, "constraint failed at %d in [%s]", pc, p.zSql)
	}
	rc = p.Halt()
	assert( rc == SQLITE_BUSY || rc == SQLITE_OK || rc == SQLITE_ERROR )
	if rc == SQLITE_BUSY {
		p.rc = rc
	} else {
		assert( rc == SQLITE_OK || p.rc == SQLITE_CONSTRAINT )
		assert( rc == SQLITE_OK || db.nDeferredCons > 0 )
		if p.rc != SQLITE_OK {
			rc = SQLITE_ERROR
		} else {
			rc = SQLITE_DONE
		}
	}
	goto vdbe_return
}

//	Opcode: Integer P1 P2 * * *
//	The 32-bit integer value P1 is written into register P2.
case OP_Integer:			//	out2-prerelease
	pOut.Store(pOp.p1)

//	Opcode: Int64 * P2 * P4 *
//	P4 is a pointer to a 64-bit integer value. Write that value into register P2.
case OP_Int64:				//	out2-prerelease
	assert( pOp.p4.pI64 != 0 )
	pOut.Store(*pOp.p4.pI64)

/* Opcode: Real * P2 * P4 *
**
** P4 is a pointer to a 64-bit floating point value.
** Write that value into register P2.
*/
case OP_Real: {            /* same as TK_FLOAT, out2-prerelease */
  pOut.flags = MEM_Real;
  assert( !math.IsNaN(*pOp.p4.pReal) )
  pOut.r = *pOp.p4.pReal;
  break;
}

//	Opcode: String8 * P2 * P4 *
//	P4 points to a nul terminated UTF-8 string. This opcode is transformed into an OP_String before it is executed for the first time.
case OP_String8:					//	same as TK_STRING, out2-prerelease
	assert( pOp.p4.z != "" )
	pOp.opcode = OP_String
	pOp.p1 = len(pOp.p4.z)
	if pOp.p1 < 0 {
		goto too_big
	}
	fallthrough
  
//	Opcode: String P1 P2 * P4 *
//	The string value P4 of length P1 (bytes) is stored in register P2.
case OP_String:				//	out2-prerelease
	assert( pOp.p4.z != "" )
	pOut.flags = MEM_Str | MEM_Static | MEM_Term
	pOut.z = pOp.p4.z
	pOut.n = pOp.p1
	pOut.enc = encoding
	UPDATE_MAX_BLOBSIZE(pOut)

//	Opcode: Null * P2 P3 * *
//	Write a NULL into registers P2. If P3 greater than P2, then also write NULL into register P3 and ever register in between P2 and P3. If P3 is less than P2 (typically P3 is zero) then only register P2 is set to NULL
case OP_Null:				//	out2-prerelease
	u.ab.cnt = pOp.p3 - pOp.p2
	assert( pOp.p3 <= p.nMem )
	pOut.Value = nil
	for u.ab.cnt > 0 {
		pOut++
		memAboutToChange(p, pOut)
		VdbeMemRelease(pOut)
		pOut.Value = nil
		u.ab.cnt--
	}

//	Opcode: Blob P1 P2 * P4
//	P4 points to a blob of data P1 bytes long. Store this blob in register P2.
case OP_Blob:						//	out2-prerelease
	assert( pOp.p1 <= SQLITE_MAX_LENGTH )
	pOut.SetStr(pOp.p4.z[:pOp.p1], 0, 0)
	pOut.enc = encoding
	UPDATE_MAX_BLOBSIZE(pOut);

//	Opcode: Variable P1 P2 * P4 *
//	Transfer the values of bound parameter P1 into register P2
//	If the parameter is named, then its name appears in P4 and P3 == 1. The P4 value is used by sqlite3_bind_parameter_name().
case OP_Variable:					//	out2-prerelease
	assert( pOp.p1 > 0 && pOp.p1 <= p.nVar )
	assert( pOp.p4.z == "" || pOp.p4.z == p.azVar[pOp.p1 - 1] )
	u.ac.pVar = &p.aVar[pOp.p1 - 1]
	if u.ac.pVar.VdbeMemTooBig() {
		goto too_big
	}
	sqlite3VdbeMemShallowCopy(pOut, u.ac.pVar, MEM_Static)
	UPDATE_MAX_BLOBSIZE(pOut)

/* Opcode: Move P1 P2 P3 * *
**
** Move the values in register P1..P1+P3-1 over into
** registers P2..P2+P3-1.  Registers P1..P1+P1-1 are
** left holding a NULL.  It is an error for register ranges
** P1..P1+P3-1 and P2..P2+P3-1 to overlap.
*/
case OP_Move: {
  u.ad.n = pOp.p3;
  u.ad.p1 = pOp.p1;
  u.ad.p2 = pOp.p2;
  assert( u.ad.n>0 && u.ad.p1>0 && u.ad.p2>0 );
  assert( u.ad.p1+u.ad.n<=u.ad.p2 || u.ad.p2+u.ad.n<=u.ad.p1 );

  pIn1 = &aMem[u.ad.p1];
  pOut = &aMem[u.ad.p2];
  while( u.ad.n-- ){
    assert( pOut<=&aMem[p.nMem] );
    assert( pIn1<=&aMem[p.nMem] );
    assert( memIsValid(pIn1) );
    memAboutToChange(p, pOut);
    u.ad.zMalloc = pOut.zMalloc;
    pOut.zMalloc = 0;
    sqlite3VdbeMemMove(pOut, pIn1);
    pIn1.zMalloc = u.ad.zMalloc;
    REGISTER_TRACE(u.ad.p2++, pOut);
    pIn1++;
    pOut++;
  }
  break;
}

/* Opcode: Copy P1 P2 * * *
**
** Make a copy of register P1 into register P2.
**
** This instruction makes a deep copy of the value.  A duplicate
** is made of any string or blob constant.  See also OP_SCopy.
*/
case OP_Copy: {             /* in1, out2 */
  pIn1 = &aMem[pOp.p1];
  pOut = &aMem[pOp.p2];
  assert( pOut!=pIn1 );
  sqlite3VdbeMemShallowCopy(pOut, pIn1, MEM_Ephem);
  Deephemeralize(pOut);
  REGISTER_TRACE(pOp.p2, pOut);
  break;
}

/* Opcode: SCopy P1 P2 * * *
**
** Make a shallow copy of register P1 into register P2.
**
** This instruction makes a shallow copy of the value.  If the value
** is a string or blob, then the copy is only a pointer to the
** original and hence if the original changes so will the copy.
** Worse, if the original is deallocated, the copy becomes invalid.
** Thus the program must guarantee that the original will not change
** during the lifetime of the copy.  Use OP_Copy to make a complete
** copy.
*/
case OP_SCopy: {            /* in1, out2 */
  pIn1 = &aMem[pOp.p1];
  pOut = &aMem[pOp.p2];
  assert( pOut!=pIn1 );
  sqlite3VdbeMemShallowCopy(pOut, pIn1, MEM_Ephem);
  REGISTER_TRACE(pOp.p2, pOut);
  break;
}

/* Opcode: ResultRow P1 P2 * * *
**
** The registers P1 through P1+P2-1 contain a single row of
** results. This opcode causes the sqlite3_step() call to terminate
** with an SQLITE_ROW return code and it sets up the sqlite3_stmt
** structure to provide access to the top P1 values as the result
** row.
*/
case OP_ResultRow: {
  assert( p.nResColumn==pOp.p2 );
  assert( pOp.p1>0 );
  assert( pOp.p1+pOp.p2<=p.nMem+1 );

  /* If this statement has violated immediate foreign key constraints, do
  ** not return the number of rows modified. And do not RELEASE the statement
  ** transaction. It needs to be rolled back.  */
  if( SQLITE_OK!=(rc = p.CheckFk(0)) ){
    assert( db.flags&SQLITE_CountRows );
    assert( p.usesStmtJournal );
    break;
  }

  /* If the SQLITE_CountRows flag is set in sqlite3.flags mask, then
  ** DML statements invoke this opcode to return the number of rows
  ** modified to the user. This is the only way that a VM that
  ** opens a statement transaction may invoke this opcode.
  **
  ** In case this is such a statement, close any statement transaction
  ** opened by this VM before returning control to the user. This is to
  ** ensure that statement-transactions are always nested, not overlapping.
  ** If the open statement-transaction is not closed here, then the user
  ** may step another VM that opens its own statement transaction. This
  ** may lead to overlapping statement transactions.
  **
  ** The statement transaction is never a top-level transaction.  Hence
  ** the RELEASE call below can never fail.
  */
  assert( p.iStatement==0 || db.flags&SQLITE_CountRows );
  rc = p.CloseStatement(SAVEPOINT_RELEASE)
  if( rc!=SQLITE_OK ){
    break;
  }

  /* Invalidate all ephemeral cursor row caches */
  p.cacheCtr = (p.cacheCtr + 2)|1;

  /* Make sure the results of the current row are \000 terminated
  ** and have an assigned type.  The results are de-ephemeralized as
  ** a side effect.
  */
  u.ae.pMem = p.pResultSet = &aMem[pOp.p1];
  for(u.ae.i=0; u.ae.i<pOp.p2; u.ae.i++){
    assert( memIsValid(&u.ae.pMem[u.ae.i]) );
    Deephemeralize(&u.ae.pMem[u.ae.i]);
    assert( (u.ae.pMem[u.ae.i].flags & MEM_Ephem)==0
            || (u.ae.pMem[u.ae.i].flags & (MEM_Str|MEM_Blob))==0 );
    sqlite3VdbeMemNulTerminate(&u.ae.pMem[u.ae.i]);
    u.ae.pMem[u.ae.i].StoreType()
    REGISTER_TRACE(pOp.p1+u.ae.i, &u.ae.pMem[u.ae.i]);
  }
  if( db.mallocFailed ) goto no_mem;

  /* Return SQLITE_ROW
  */
  p.pc = pc + 1;
  rc = SQLITE_ROW;
  goto vdbe_return;
}

//	Opcode: Concat P1 P2 P3 * *
//	Add the text in register P1 onto the end of the text in register P2 and store the result in register P3. If either the P1 or P2 text are NULL then store NULL in P3.
//			P3 = P2 || P1
//	It is illegal for P1 and P3 to be the same register. Sometimes, if P3 is the same register as P2, the implementation is able to avoid a memcpy().
case OP_Concat:					//	same as TK_CONCAT, in1, in2, out3
	pIn1 = &aMem[pOp.p1]
	pIn2 = &aMem[pOp.p2]
	pOut = &aMem[pOp.p3]
	assert( pIn1 != pOut )
	if pIn1.Value != nil && pIn2.Value != nil {					//	(pIn1.flags | pIn2.flags) & MEM_Null == 0 {
		if ExpandBlob(pIn1) || ExpandBlob(pIn2) {
			goto no_mem
		}
		Stringify(pIn1, encoding)
		Stringify(pIn2, encoding)
		u.af.nByte = pIn1.n + pIn2.n
		if u.af.nByte < 0 {
			goto too_big;
		}
		pOut.SetTypeFlag(MEM_Str)
		if sqlite3VdbeMemGrow(pOut, int(u.af.nByte + 2), pOut == pIn2) != SQLITE_OK {
			goto no_mem
		}
		if pOut != pIn2 {
			memcpy(pOut.z, pIn2.z, pIn2.n)
		}
		memcpy(&pOut.z[pIn2.n], pIn1.z, pIn1.n)
		pOut.z[u.af.nByte] = 0
		pOut.z[u.af.nByte + 1] = 0
		pOut.flags |= MEM_Term
		pOut.n = int(u.af.nByte)
		pOut.enc = encoding
		UPDATE_MAX_BLOBSIZE(pOut)
	} else {
		pOut.SetNull()
	}

//	Opcode: Add P1 P2 P3 * *
//	Add the value in register P1 to the value in register P2 and store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: Multiply P1 P2 P3 * *
//	Multiply the value in register P1 by the value in register P2 and store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: Subtract P1 P2 P3 * *
//	Subtract the value in register P1 from the value in register P2 and store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: Divide P1 P2 P3 * *
//	Divide the value in register P1 by the value in register P2 and store the result in register P3 (P3=P2/P1). If the value in register P1 is zero, then the result is NULL. If either input is NULL, the result is NULL.
//
//	Opcode: Remainder P1 P2 P3 * *
//	Compute the remainder after integer division of the value in register P1 by the value in register P2 and store the result in P3. If the value in register P2 is zero the result is NULL. If either operand is NULL, the result is NULL.
case OP_Add:			fallthrough			//	same as TK_PLUS, in1, in2, out3
case OP_Subtract:		fallthrough			//	same as TK_MINUS, in1, in2, out3
case OP_Multiply:		fallthrough			//	same as TK_STAR, in1, in2, out3
case OP_Divide:			fallthrough			//	same as TK_SLASH, in1, in2, out3
case OP_Remainder:							//	same as TK_REM, in1, in2, out3
	pIn1 = &aMem[pOp.p1]
	pIn1.applyNumericAffinity()
	pIn2 = &aMem[pOp.p2]
	pIn2.applyNumericAffinity()
	pOut = &aMem[pOp.p3]
	u.ag.flags = pIn1.flags | pIn2.flags
	if u.ag.flags & MEM_Null != 0 {
		goto arithmetic_result_is_null
	}
	if pIn1.flags & pIn2.flags == {
		u.ag.iA = pIn1.Integer()
		u.ag.iB = pIn2.Integer()
		switch pOp.opcode {
		case OP_Add:
			if sqlite3AddInt64(&u.ag.iB,u.ag.iA) {
				goto fp_math
			}
		case OP_Subtract:
			if sqlite3SubInt64(&u.ag.iB,u.ag.iA) {
				goto fp_math
			}
		case OP_Multiply:
			if sqlite3MulInt64(&u.ag.iB,u.ag.iA) {
				goto fp_math
			}
		case OP_Divide:
			switch {
			case u.ag.iA == 0:
				goto arithmetic_result_is_null
			case u.ag.iA == -1 && u.ag.iB == SMALLEST_INT64:
				goto fp_math
			}
			u.ag.iB /= u.ag.iA
			break
		default:
			switch {
			case u.ag.iA == 0:
				goto arithmetic_result_is_null
			case u.ag.iA == -1:
				u.ag.iA = 1
			}
			u.ag.iB %= u.ag.iA
		}
		pOut.Store(u.ag.iB)
	} else {
fp_math:
		u.ag.rA = sqlite3VdbeRealValue(pIn1)
		u.ag.rB = sqlite3VdbeRealValue(pIn2)
		switch pOp.opcode {
		case OP_Add:
			u.ag.rB += u.ag.rA
		case OP_Subtract:
			u.ag.rB -= u.ag.rA
		case OP_Multiply:
			u.ag.rB *= u.ag.rA
		case OP_Divide:
			u.ag.rB /= u.ag.rA
		default:
			u.ag.iA = int64(u.ag.rA)
			u.ag.iB = int64(u.ag.rB)
			switch {
			case u.ag.iA == 0:
				goto arithmetic_result_is_null
			case u.ag.iA == -1:
				u.ag.iA = 1
			}
			u.ag.rB = (double)(u.ag.iB % u.ag.iA);
		}
		if math.IsNaN(u.ag.rB) {
			goto arithmetic_result_is_null
		}
		pOut.r = u.ag.rB;
		pOut.SetTypeFlag(MEM_Real)
		if u.ag.flags & MEM_Real == 0 {
			pOut.IntegerAffinity()
		}
	}
	break

arithmetic_result_is_null:
	pOut.SetNull()

//	Opcode: CollSeq P1 * * P4
//	P4 is a pointer to a CollSeq struct. If the next call to a user function or aggregate calls sqlite3GetFuncCollSeq(), this collation sequence will be returned. This is used by the built-in min(), max() and nullif() functions.
//	If P1 is not zero, then it is a register that a subsequent min() or max() aggregate will set to 1 if the current row is not the minimum or maximum. The P1 register is initialized to 0 by this instruction.
//	The interface used by the implementation of the aforementioned functions to retrieve the collation sequence set by this opcode is not available publicly, only to user functions defined in func.c.
case OP_CollSeq:
	assert( pOp.p4type == P4_COLLSEQ )
	if pOp.p1 != 0 {
		aMem[pOp.p1].SetInt64(0)
	}

//	Opcode: Function P1 P2 P3 P4 P5
//	Invoke a user function (P4 is a pointer to a Function structure that defines the function) with P5 arguments taken from register P2 and successors. The result of the function is stored in register P3. Register P3 must not be one of the function inputs.
//	P1 is a 32-bit bitmask indicating whether or not each argument to the function was determined to be constant at compile time. If the first argument was constant then bit 0 of P1 is set. This is used to determine whether meta data associated with a user function argument using the sqlite3_set_auxdata() API may be safely retained until the next invocation of this opcode.
//	See also: AggStep and AggFinal
case OP_Function:
	u.ah.n = pOp.p5
	u.ah.apVal = p.apArg
	assert( u.ah.apVal || u.ah.n == 0 )
	assert( pOp.p3 > 0 && pOp.p3 <= p.nMem )
	pOut = &aMem[pOp.p3]
	memAboutToChange(p, pOut)

	assert( u.ah.n == 0 || (pOp.p2 > 0 && pOp.p2 + u.ah.n <= p.nMem + 1) )
	assert( pOp.p3 < pOp.p2 || pOp.p3 >= pOp.p2 + u.ah.n )
	u.ah.pArg = &aMem[pOp.p2]
	for u.ah.i = 0; u.ah.i < u.ah.n; u.ah.i++, u.ah.pArg++ {
		assert( memIsValid(u.ah.pArg) )
		u.ah.apVal[u.ah.i] = u.ah.pArg
		Deephemeralize(u.ah.pArg)
		u.ah.pArg.StoreType()
		REGISTER_TRACE(pOp.p2 + u.ah.i, u.ah.pArg)
	}

	assert( pOp.p4type == P4_FUNCDEF || pOp.p4type == P4_VDBEFUNC )
	if pOp.p4type == P4_FUNCDEF {
		u.ah.ctx.pFunc = pOp.p4.pFunc
		u.ah.ctx.pVdbeFunc = 0
	} else {
		u.ah.ctx.pVdbeFunc = (VdbeFunc*)(pOp.p4.pVdbeFunc)
		u.ah.ctx.pFunc = u.ah.ctx.pVdbeFunc.pFunc
	}

	u.ah.ctx.s.flags = MEM_Null
	u.ah.ctx.s.db = db
	u.ah.ctx.s.xDel = nil
	u.ah.ctx.s.zMalloc = nil

	//	The output cell may already have a buffer allocated. Move the pointer to u.ah.ctx.s so in case the user-function can use the already allocated buffer instead of allocating a new one.
	sqlite3VdbeMemMove(&u.ah.ctx.s, pOut)
	u.ah.ctx.s.Value = nil

	u.ah.ctx.isError = false
	if u.ah.ctx.pFunc.flags & SQLITE_FUNC_NEEDCOLL {
		assert( &pOp > &aOp )
		assert( pOp[-1].p4type == P4_COLLSEQ )
		assert( pOp[-1].opcode == OP_CollSeq )
		u.ah.ctx.pColl = pOp[-1].p4.pColl
	}
	db.lastRowid = lastRowid
	(*u.ah.ctx.pFunc.xFunc)(&u.ah.ctx, u.ah.n, u.ah.apVal)			//	IMP: R-24505-23230
	lastRowid = db.lastRowid

	//	If any auxiliary data functions have been called by this user function, immediately call the destructor for any non-static values.
	if u.ah.ctx.pVdbeFunc {
		sqlite3VdbeDeleteAuxData(u.ah.ctx.pVdbeFunc, pOp.p1)
		pOp.p4.pVdbeFunc = u.ah.ctx.pVdbeFunc
		pOp.p4type = P4_VDBEFUNC
	}

	if db.mallocFailed {
		//	Even though a malloc() has failed, the implementation of the user function may have called an sqlite3_result_XXX() function to return a value. The following call releases any resources associated with such a value.
		u.ah.ctx.s.Release()
		goto no_mem;
	}

	//	If the function returned an error, throw an exception
	if u.ah.ctx.isError {
		p.zErrMsg = fmt.Sprintf("%v", sqlite3_value_text(&u.ah.ctx.s))
		rc = u.ah.ctx.isError
	}

	//	Copy the result of the function into register P3
	sqlite3VdbeChangeEncoding(&u.ah.ctx.s, encoding)
	sqlite3VdbeMemMove(pOut, &u.ah.ctx.s)
	if pOut.VdbeMemTooBig() {
		goto too_big;
	}

	REGISTER_TRACE(pOp.p3, pOut)
	UPDATE_MAX_BLOBSIZE(pOut)

//	Opcode: BitAnd P1 P2 P3 * *
//	Take the bit-wise AND of the values in register P1 and P2 and store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: BitOr P1 P2 P3 * *
//	Take the bit-wise OR of the values in register P1 and P2 and store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: ShiftLeft P1 P2 P3 * *
//	Shift the integer value in register P2 to the left by the number of bits specified by the integer in register P1. Store the result in register P3. If either input is NULL, the result is NULL.
//
//	Opcode: ShiftRight P1 P2 P3 * *
//	Shift the integer value in register P2 to the right by the number of bits specified by the integer in register P1. Store the result in register P3. If either input is NULL, the result is NULL.
case OP_BitAnd:			fallthrough		//	same as TK_BITAND, in1, in2, out3
case OP_BitOr:			fallthrough		//	same as TK_BITOR, in1, in2, out3
case OP_ShiftLeft:		fallthrough		//	same as TK_LSHIFT, in1, in2, out3
case OP_ShiftRight:						//	same as TK_RSHIFT, in1, in2, out3
	pIn1 = &aMem[pOp.p1]
	pIn2 = &aMem[pOp.p2]
	pOut = &aMem[pOp.p3]
	if pIn1.Value != nil && pIn2.Value != nil {				//	(pIn1.flags | pIn2.flags) & MEM_Null == 0 {
		u.ai.iA = pIn2.IntValue()
		u.ai.iB = pIn1.IntValue()
		u.ai.op = pOp.opcode
		switch {
		case u.ai.op == OP_BitAnd:
			u.ai.iA &= u.ai.iB
		case u.ai.op == OP_BitOr:
			u.ai.iA |= u.ai.iB
		case u.ai.iB != 0:
			assert( u.ai.op == OP_ShiftRight || u.ai.op == OP_ShiftLeft )

			//	If shifting by a negative amount, shift in the other direction
			if u.ai.iB < 0 {
				assert( OP_ShiftRight == OP_ShiftLeft + 1 )
				u.ai.op = 2 * OP_ShiftLeft + 1 - u.ai.op
				if u.ai.iB > -64 {
					u.ai.iB = -u.ai.iB
				} else {
					u.ai.iB = 64
				}
			}

			if u.ai.iB >= 64 {
				u.ai.iA = (u.ai.iA >= 0 || u.ai.op == OP_ShiftLeft) ? 0 : -1
			} else {
				memcpy(&u.ai.uA, &u.ai.iA, sizeof(u.ai.uA))
				if u.ai.op == OP_ShiftLeft {
					u.ai.uA <<= u.ai.iB
				} else {
					u.ai.uA >>= u.ai.iB
					//	Sign-extend on a right shift of a negative number
					if u.ai.iA < 0 {
						u.ai.uA |= ((((uint64)0xffffffff)<<32)|0xffffffff) << (64-u.ai.iB)
					}
				}
				memcpy(&u.ai.iA, &u.ai.uA, sizeof(u.ai.iA))
			}
		}
		pOut.Store(u.ai.iA)
	} else {
		pOut.SetNull()
	}

//	Opcode: AddImm  P1 P2 * * *
//	Add the constant P2 to the value in register P1. The result is always an integer.
//	To force any register to be an integer, just add 0.
case OP_AddImm:				//	in1
	pIn1 = &aMem[pOp.p1]
	memAboutToChange(p, pIn1)
	pIn1.Integerify()
	pIn1.AddInt(pOp.p2)

//	Opcode: MustBeInt P1 P2 * * *
//	Force the value in register P1 to be an integer. If the value in P1 is not an integer and cannot be converted into an integer without data loss, then jump immediately to P2, or if P2 == 0 raise an SQLITE_MISMATCH exception.
case OP_MustBeInt:					//	jump, in1
	pIn1 = &aMem[pOp.p1]
	applyAffinity(pIn1, SQLITE_AFF_NUMERIC, encoding)
	if _, ok := pIn1.Value.(int64); !ok {
		if pOp.p2 == 0 {
			rc = SQLITE_MISMATCH
			goto abort_due_to_error
		} else {
			pc = pOp.p2 - 1
		}
	}

//	Opcode: RealAffinity P1 * * * *
//	If register P1 holds an integer convert it to a real value.
//	This opcode is used when extracting information from a column that has REAL affinity. Such column values may still be stored as integers, for space efficiency, but after extraction we want them to have only a real value.
case OP_RealAffinity:				//	in1
	pIn1 = &aMem[pOp.p1]
	if _, ok := pIn1.(int64); ok {
		pIn1.Realify()
	}

#ifndef SQLITE_OMIT_CAST
//	Opcode: ToText P1 * * * *
//	Force the value in register P1 to be text. If the value is numeric, convert it to a string using the equivalent of printf(). Blob values are unchanged and are afterwards simply interpreted as text.
//	A NULL value is not changed by this routine. It remains NULL.
case OP_ToText:						//	same as TK_TO_TEXT, in1
	pIn1 = &aMem[pOp.p1]
	memAboutToChange(p, pIn1)
	if pIn1.Value != nil {
		assert( MEM_Str == (MEM_Blob >> 3) )
		pIn1.flags |= (pIn1.flags & MEM_Blob) >> 3
		applyAffinity(pIn1, SQLITE_AFF_TEXT, encoding)
		rc = ExpandBlob(pIn1)
		assert( pIn1.flags & MEM_Str || db.mallocFailed )
		pIn1.flags &= ~( | MEM_Real | MEM_Blob)
		UPDATE_MAX_BLOBSIZE(pIn1)
	}

//	Opcode: ToBlob P1 * * * *
//	Force the value in register P1 to be a BLOB. If the value is numeric, convert it to a string first. Strings are simply reinterpreted as blobs with no change to the underlying data.
//	A NULL value is not changed by this routine. It remains NULL.
case OP_ToBlob:							//	same as TK_TO_BLOB, in1
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value != nil {
		if pIn1.flags & MEM_Blob == 0 {
			applyAffinity(pIn1, SQLITE_AFF_TEXT, encoding)
			assert( pIn1.flags & MEM_Str || db.mallocFailed )
			pIn1.SetTypeFlag(MEM_Blob)
		} else {
			pIn1.flags &= ~(MEM_TypeMask & ~MEM_Blob)
		}
		UPDATE_MAX_BLOBSIZE(pIn1)
	}

//	Opcode: ToNumeric P1 * * * *
//	Force the value in register P1 to be numeric (either an integer or a floating-point number.) If the value is text or blob, try to convert it to an using the equivalent of atoi() or atof() and store 0 if no such conversion is possible.
//	A NULL value is not changed by this routine. It remains NULL.
case OP_ToNumeric:						//	same as TK_TO_NUMERIC, in1
	pIn1 = &aMem[pOp.p1]
	pIn1.Numerify()
#endif /* SQLITE_OMIT_CAST */

//	Opcode: ToInt P1 * * * *
//	Force the value in register P1 to be an integer. If the value is currently a real number, drop its fractional part. If the value is text or blob, try to convert it to an integer using the equivalent of atoi() and store 0 if no such conversion is possible.
//	A NULL value is not changed by this routine. It remains NULL.
case OP_ToInt:							//	same as TK_TO_INT, in1
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value != nil {
		pIn1.Integerify()
	}

#if !defined(SQLITE_OMIT_CAST)
//	Opcode: ToReal P1 * * * *
//	Force the value in register P1 to be a floating point number. If The value is currently an integer, convert it. If the value is text or blob, try to convert it to an integer using the equivalent of atoi() and store 0.0 if no such conversion is possible.
//	A NULL value is not changed by this routine. It remains NULL.
case OP_ToReal:							//	same as TK_TO_REAL, in1
	pIn1 = &aMem[pOp.p1]
	memAboutToChange(p, pIn1)
	if pIn1.Values != nil {
		pIn1.Realify()
	}
#endif /* !defined(SQLITE_OMIT_CAST) */

//	Opcode: Lt P1 P2 P3 P4 P5
//	Compare the values in register P1 and P3.  If reg(P3)<reg(P1) then jump to address P2.
//	If the SQLITE_JUMPIFNULL bit of P5 is set and either reg(P1) or reg(P3) is NULL then take the jump. If the SQLITE_JUMPIFNULL bit is clear then fall through if either operand is NULL.
//	The SQLITE_AFF_MASK portion of P5 must be an affinity character - SQLITE_AFF_TEXT, SQLITE_AFF_INTEGER, and so forth. An attempt is made to coerce both inputs according to this affinity before the comparison is made. If the SQLITE_AFF_MASK is 0x00, then numeric affinity is used. Note that the affinity conversions are stored back into the input registers P1 and P3. So this opcode can cause persistent changes to registers P1 and P3.
//	Once any conversions have taken place, and neither value is NULL, the values are compared. If both values are blobs then memcmp() is used to determine the results of the comparison. If both values are text, then the appropriate collating function specified in P4 is  used to do the comparison. If P4 is not specified then memcmp() is used to compare text string. If both values are numeric, then a numeric comparison is used. If the two values are of different types, then numbers are considered less than strings and strings are considered less than blobs.
//	If the SQLITE_STOREP2 bit of P5 is set, then do not jump. Instead, store a boolean result (either 0, or 1, or NULL) in register P2.
//
//	Opcode: Ne P1 P2 P3 P4 P5
//	This works just like the Lt opcode except that the jump is taken if the operands in registers P1 and P3 are not equal. See the Lt opcode for additional information.
//	If SQLITE_NULLEQ is set in P5 then the result of comparison is always either true or false and is never NULL. If both operands are NULL then the result of comparison is false. If either operand is NULL then the result is true. If neither operand is NULL the result is the same as it would be if the SQLITE_NULLEQ flag were omitted from P5.
//
//	Opcode: Eq P1 P2 P3 P4 P5
//	This works just like the Lt opcode except that the jump is taken if the operands in registers P1 and P3 are equal. See the Lt opcode for additional information.
//	If SQLITE_NULLEQ is set in P5 then the result of comparison is always either true or false and is never NULL. If both operands are NULL then the result of comparison is true. If either operand is NULL then the result is false. If neither operand is NULL the result is the same as it would be if the SQLITE_NULLEQ flag were omitted from P5.
//
//	Opcode: Le P1 P2 P3 P4 P5
//	This works just like the Lt opcode except that the jump is taken if the content of register P3 is less than or equal to the content of register P1. See the Lt opcode for additional information.
//
//	Opcode: Gt P1 P2 P3 P4 P5
//	This works just like the Lt opcode except that the jump is taken if the content of register P3 is greater than the content of register P1. See the Lt opcode for additional information.
//
//	Opcode: Ge P1 P2 P3 P4 P5
//	This works just like the Lt opcode except that the jump is taken if the content of register P3 is greater than or equal to the content of register P1. See the Lt opcode for additional information.
case OP_Eq:		fallthrough		//	same as TK_EQ, jump, in1, in3
case OP_Ne:		fallthrough		//	same as TK_NE, jump, in1, in3
case OP_Lt:		fallthrough		//	same as TK_LT, jump, in1, in3
case OP_Le:		fallthrough		//	same as TK_LE, jump, in1, in3
case OP_Gt:		fallthrough		//	same as TK_GT, jump, in1, in3
case OP_Ge:						//	same as TK_GE, jump, in1, in3
	pIn1 = &aMem[pOp.p1]
	pIn3 = &aMem[pOp.p3]
	u.aj.flags1 = pIn1.flags
	u.aj.flags3 = pIn3.flags
	if pIn1.Value == nil || pIn3.Value == nil {
		//	One or both operands are NULL
		if pOp.p5 & SQLITE_NULLEQ {
			//	If SQLITE_NULLEQ is set (which will only happen if the operator is OP_Eq or OP_Ne) then take the jump or not depending on whether or not both operands are null.
			assert( pOp.opcode == OP_Eq || pOp.opcode == OP_Ne )
			u.aj.res = pIn1.Value != nil || pIn3.Value != nil					//     (u.aj.flags1 & u.aj.flags3 & MEM_Null) == 0
		} else {
			//	SQLITE_NULLEQ is clear and at least one operand is NULL, then the result is always NULL. The jump is taken if the SQLITE_JUMPIFNULL bit is set.
			switch {
			case pOp.p5 & SQLITE_STOREP2:
				pOut = &aMem[pOp.p2]
				pOut.Value = nil
				REGISTER_TRACE(pOp.p2, pOut)
			case pOp.p5 & SQLITE_JUMPIFNULL:
				pc = pOp.p2-1
			}
			break
		}
	} else {
		//	Neither operand is NULL. Do a comparison.
		if u.aj.affinity = pOp.p5 & SQLITE_AFF_MASK; u.aj.affinity != 0 {
			applyAffinity(pIn1, u.aj.affinity, encoding)
			applyAffinity(pIn3, u.aj.affinity, encoding)
			if db.mallocFailed {
				goto no_mem
			}
		}
		assert( pOp.p4type == P4_COLLSEQ || pOp.p4.pColl == 0 )
		ExpandBlob(pIn1)
		ExpandBlob(pIn3)
		u.aj.res = sqlite3MemCompare(pIn3, pIn1, pOp.p4.pColl)
	}
	switch pOp.opcode {
    case OP_Eq:
		u.aj.res = u.aj.res == 0
    case OP_Ne:
		u.aj.res = u.aj.res != 0
    case OP_Lt:
		u.aj.res = u.aj.res < 0
    case OP_Le:
		u.aj.res = u.aj.res <= 0
    case OP_Gt:
		u.aj.res = u.aj.res > 0
    default:
		u.aj.res = u.aj.res >= 0
	}
	if pOp.p5 & SQLITE_STOREP2 != 0 {
		pOut = &aMem[pOp.p2]
		memAboutToChange(p, pOut)
		pOut.Store(u.aj.res)
		REGISTER_TRACE(pOp.p2, pOut)
	} else if u.aj.res {
		pc = pOp.p2 - 1
	}

	//	Undo any changes made by applyAffinity() to the input registers.
	pIn1.flags = (pIn1.flags&~MEM_TypeMask) | (u.aj.flags1&MEM_TypeMask)
	pIn3.flags = (pIn3.flags&~MEM_TypeMask) | (u.aj.flags3&MEM_TypeMask)

//	Opcode: Permutation * * * P4 *
//	Set the permutation used by the OP_Compare operator to be the array of integers in P4.
//	The permutation is only valid until the next OP_Permutation, OP_Compare, OP_Halt, or OP_ResultRow. Typically the OP_Permutation should occur immediately prior to the OP_Compare.
case OP_Permutation:
	assert( pOp.p4type == P4_INTARRAY )
	assert( pOp.p4.ai )
	aPermute = pOp.p4.ai

/* Opcode: Compare P1 P2 P3 P4 *
**
** Compare two vectors of registers in reg(P1)..reg(P1+P3-1) (call this
** vector "A") and in reg(P2)..reg(P2+P3-1) ("B").  Save the result of
** the comparison for use by the next OP_Jump instruct.
**
** P4 is a KeyInfo structure that defines collating sequences and sort
** orders for the comparison.  The permutation applies to registers
** only.  The KeyInfo elements are used sequentially.
**
** The comparison is a sort comparison, so NULLs compare equal,
** NULLs are less than numbers, numbers are less than strings,
** and strings are less than blobs.
*/
case OP_Compare: {
  u.ak.n = pOp.p3;
  u.ak.pKeyInfo = pOp.p4.pKeyInfo;
  assert( u.ak.n>0 );
  assert( u.ak.pKeyInfo!=0 );
  u.ak.p1 = pOp.p1;
  u.ak.p2 = pOp.p2;
  for(u.ak.i=0; u.ak.i<u.ak.n; u.ak.i++){
    u.ak.idx = aPermute ? aPermute[u.ak.i] : u.ak.i;
    assert( memIsValid(&aMem[u.ak.p1+u.ak.idx]) );
    assert( memIsValid(&aMem[u.ak.p2+u.ak.idx]) );
    REGISTER_TRACE(u.ak.p1+u.ak.idx, &aMem[u.ak.p1+u.ak.idx]);
    REGISTER_TRACE(u.ak.p2+u.ak.idx, &aMem[u.ak.p2+u.ak.idx]);
    assert( u.ak.i<u.ak.pKeyInfo.nField );
    u.ak.pColl = u.ak.pKeyInfo.Collations[u.ak.i];
    u.ak.bRev = u.ak.pKeyInfo.aSortOrder[u.ak.i];
    iCompare = sqlite3MemCompare(&aMem[u.ak.p1+u.ak.idx], &aMem[u.ak.p2+u.ak.idx], u.ak.pColl);
    if( iCompare ){
      if( u.ak.bRev ) iCompare = -iCompare;
      break;
    }
  }
  aPermute = 0;
  break;
}

/* Opcode: Jump P1 P2 P3 * *
**
** Jump to the instruction at address P1, P2, or P3 depending on whether
** in the most recent OP_Compare instruction the P1 vector was less than
** equal to, or greater than the P2 vector, respectively.
*/
case OP_Jump: {             /* jump */
  if( iCompare<0 ){
    pc = pOp.p1 - 1;
  }else if( iCompare==0 ){
    pc = pOp.p2 - 1;
  }else{
    pc = pOp.p3 - 1;
  }
  break;
}

//	Opcode: And P1 P2 P3 * *
//	Take the logical AND of the values in registers P1 and P2 and write the result into register P3.
//	If either P1 or P2 is 0 (false) then the result is 0 even if the other input is NULL. A NULL and true or two NULLs give a NULL output.
//
//	Opcode: Or P1 P2 P3 * *
//	Take the logical OR of the values in register P1 and P2 and store the answer in register P3.
//	If either P1 or P2 is nonzero (true) then the result is 1 (true) even if the other input is NULL. A NULL and false or two NULLs give a NULL output.
case OP_And:		fallthrough			//	same as TK_AND, in1, in2, out3
case OP_Or:								//	same as TK_OR, in1, in2, out3
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value == nil {
		u.al.v1 = 2
	} else {
		u.al.v1 = pIn1.IntValue() != 0
	}
	pIn2 = &aMem[pOp.p2]
	if pIn2.Value == nil {
		u.al.v2 = 2
	} else {
		u.al.v2 = pIn2.IntValue() != 0
	}
	if pOp.opcode == OP_And {
		and_logic := []byte{ 0, 0, 0, 0, 1, 2, 0, 2, 2 }
		u.al.v1 = and_logic[u.al.v1 * 3 + u.al.v2]
	} else {
		or_logic := []byte{ 0, 1, 2, 1, 1, 1, 2, 1, 2 }
		u.al.v1 = or_logic[u.al.v1 * 3 + u.al.v2]
	}
	pOut = &aMem[pOp.p3]
	if u.al.v1 == 2 {
		pOut.Value = nil
	} else {
		pOut.Store(u.al.v1)
	}

//	Opcode: Not P1 P2 * * *
//	Interpret the value in register P1 as a boolean value. Store the boolean complement in register P2. If the value in register P1 is NULL, then a NULL is stored in P2.
case OP_Not:					//	same as TK_NOT, in1, out2
	pIn1 = &aMem[pOp.p1]
	pOut = &aMem[pOp.p2]
	if pIn1.Value == nil {
		pOut.SetNull()
	} else {
		if pIn1.IntValue() {
			pOut.SetInt64(0)
		} else {
			pOut.SetInt64(1)
		}
	}

//	Opcode: BitNot P1 P2 * * *
//	Interpret the content of register P1 as an integer. Store the ones-complement of the P1 value into register P2. If P1 holds a NULL then store a NULL in P2.
case OP_BitNot:					//	same as TK_BITNOT, in1, out2
	pIn1 = &aMem[pOp.p1]
	pOut = &aMem[pOp.p2]
	if pIn1.Value == nil {
		pOut.SetNull()
	} else {
		pOut.SetInt64(~pIn1.IntValue())
	}

//	Opcode: Once P1 P2 * * *
//	Check if OP_Once flag P1 is set. If so, jump to instruction P2. Otherwise, set the flag and fall through to the next instruction.
//	See also: JumpOnce
case OP_Once:					//	jump
	assert( pOp.p1 < p.nOnceFlag )
	if p.aOnceFlag[pOp.p1] {
		pc = pOp.p2 - 1
	} else {
		p.aOnceFlag[pOp.p1] = 1
	}

//	Opcode: If P1 P2 P3 * *
//	Jump to P2 if the value in register P1 is true. The value is considered true if it is numeric and non-zero. If the value in P1 is NULL then take the jump if P3 is non-zero.
//
//	Opcode: IfNot P1 P2 P3 * *
//	Jump to P2 if the value in register P1 is False. The value is considered false if it has a numeric value of zero. If the value in P1 is NULL then take the jump if P3 is zero.
case OP_If:			fallthrough		//	jump, in1
case OP_IfNot:						//	jump, in1
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value == nil {
		u.am.c = pOp.p3
	} else {
		u.am.c = sqlite3VdbeRealValue(pIn1) != 0
		if pOp.opcode == OP_IfNot {
			u.am.c = !u.am.c
		}
	}
	if u.am.c {
		pc = pOp.p2 - 1
	}

//	Opcode: IsNull P1 P2 * * *
//	Jump to P2 if the value in register P1 is NULL.
case OP_IsNull:				//	same as TK_ISNULL, jump, in1
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value == nil {
		pc = pOp.p2 - 1
	}

//	Opcode: NotNull P1 P2 * * *
//	Jump to P2 if the value in register P1 is not NULL.
case OP_NotNull:				//	same as TK_NOTNULL, jump, in1
	pIn1 = &aMem[pOp.p1]
	if pIn1.Value != nil {
		pc = pOp.p2 - 1
	}

/* Opcode: Column P1 P2 P3 P4 P5
**
** Interpret the data that cursor P1 points to as a structure built using
** the MakeRecord instruction.  (See the MakeRecord opcode for additional
** information about the format of the data.)  Extract the P2-th column
** from this record.  If there are less that (P2+1) 
** values in the record, extract a NULL.
**
** The value extracted is stored in register P3.
**
** If the column contains fewer than P2 fields, then extract a NULL.  Or,
** if the P4 argument is a P4_MEM use the value of the P4 argument as
** the result.
**
** If the OPFLAG_CLEARCACHE bit is set on P5 and P1 is a pseudo-table cursor,
** then the cache of the cursor is reset prior to extracting the column.
** The first OP_Column against a pseudo-table after the value of the content
** register has changed should have this bit set.
**
** If the OPFLAG_LENGTHARG and OPFLAG_TYPEOFARG bits are set on P5 when
** the result is guaranteed to only be used as the argument of a length()
** or typeof() function, respectively.  The loading of large blobs can be
** skipped for length() and all content loading can be skipped for typeof().
*/
case OP_Column: {
  u.an.p1 = pOp.p1;
  u.an.p2 = pOp.p2;
  u.an.pC = 0;
  memset(&u.an.sMem, 0, sizeof(u.an.sMem));
  assert( u.an.p1<p.nCursor );
  assert( pOp.p3>0 && pOp.p3<=p.nMem );
  u.an.pDest = &aMem[pOp.p3];
  memAboutToChange(p, u.an.pDest);
  u.an.zRec = 0;

  /* This block sets the variable u.an.payloadSize to be the total number of
  ** bytes in the record.
  **
  ** u.an.zRec is set to be the complete text of the record if it is available.
  ** The complete record text is always available for pseudo-tables
  ** If the record is stored in a cursor, the complete record text
  ** might be available in the  u.an.pC.aRow cache.  Or it might not be.
  ** If the data is unavailable,  u.an.zRec is set to NULL.
  **
  ** We also compute the number of columns in the record.  For cursors,
  ** the number of columns is stored in the VdbeCursor.nField element.
  */
  u.an.pC = p.apCsr[u.an.p1];
  assert( u.an.pC!=0 );
  assert( u.an.pC.pVtabCursor==0 );
  u.an.pCrsr = u.an.pC.pCursor;
  if( u.an.pCrsr!=0 ){
    /* The record is stored in a B-Tree */
    rc = sqlite3VdbeCursorMoveto(u.an.pC);
    if( rc ) goto abort_due_to_error;
    if( u.an.pC.nullRow ){
      u.an.payloadSize = 0;
    }else if( u.an.pC.cacheStatus==p.cacheCtr ){
      u.an.payloadSize = u.an.pC.payloadSize;
      u.an.zRec = (char*)u.an.pC.aRow;
    }else if( u.an.pC.isIndex ){
      assert( sqlite3BtreeCursorIsValid(u.an.pCrsr) );
      VVA_ONLY(rc =) sqlite3BtreeKeySize(u.an.pCrsr, &u.an.payloadSize64);
      assert( rc==SQLITE_OK );   /* True because of CursorMoveto() call above */
      /* sqlite3BtreeParseCellPtr() uses getVarint32() to extract the
      ** payload size, so it is impossible for u.an.payloadSize64 to be
      ** larger than 32 bits. */
      assert( (u.an.payloadSize64 & SQLITE_MAX_U32)==(uint64)u.an.payloadSize64 );
      u.an.payloadSize = (uint32)u.an.payloadSize64;
    }else{
      assert( sqlite3BtreeCursorIsValid(u.an.pCrsr) );
      VVA_ONLY(rc =) sqlite3BtreeDataSize(u.an.pCrsr, &u.an.payloadSize);
      assert( rc==SQLITE_OK );   /* DataSize() cannot fail */
    }
  }else if( u.an.pC.pseudoTableReg>0 ){
    u.an.pReg = &aMem[u.an.pC.pseudoTableReg];
    assert( u.an.pReg.flags & MEM_Blob );
    assert( memIsValid(u.an.pReg) );
    u.an.payloadSize = u.an.pReg.n;
    u.an.zRec = u.an.pReg.z;
    u.an.pC.cacheStatus = (pOp.p5&OPFLAG_CLEARCACHE) ? CACHE_STALE : p.cacheCtr;
    assert( u.an.payloadSize==0 || u.an.zRec!=0 );
  }else{
    /* Consider the row to be NULL */
    u.an.payloadSize = 0;
  }

  /* If u.an.payloadSize is 0, then just store a NULL.  This can happen because of
  ** nullRow or because of a corrupt database. */
  if( u.an.payloadSize==0 ){
    u.an.pDest.Value = nil
    goto op_column_out;
  }
  if u.an.payloadSize < 0 {
    goto too_big;
  }

  u.an.nField = u.an.pC.nField;
  assert( u.an.p2<u.an.nField );

  /* Read and parse the table header.  Store the results of the parse
  ** into the record header cache fields of the cursor.
  */
  u.an.aType = u.an.pC.aType;
  if( u.an.pC.cacheStatus==p.cacheCtr ){
    u.an.aOffset = u.an.pC.aOffset;
  }else{
    assert(u.an.aType);
    u.an.avail = 0;
    u.an.pC.aOffset = u.an.aOffset = &u.an.aType[u.an.nField];
    u.an.pC.payloadSize = u.an.payloadSize;
    u.an.pC.cacheStatus = p.cacheCtr;

    /* Figure out how many bytes are in the header */
    if( u.an.zRec ){
      u.an.zData = u.an.zRec;
    }else{
      if( u.an.pC.isIndex ){
        u.an.zData = (char*)sqlite3BtreeKeyFetch(u.an.pCrsr, &u.an.avail);
      }else{
        u.an.zData = (char*)sqlite3BtreeDataFetch(u.an.pCrsr, &u.an.avail);
      }
      /* If KeyFetch()/DataFetch() managed to get the entire payload,
      ** save the payload in the u.an.pC.aRow cache.  That will save us from
      ** having to make additional calls to fetch the content portion of
      ** the record.
      */
      assert( u.an.avail>=0 );
      if( u.an.payloadSize <= (uint32)u.an.avail ){
        u.an.zRec = u.an.zData;
        u.an.pC.aRow = (byte*)u.an.zData;
      }else{
        u.an.pC.aRow = 0;
      }
    }
    /* The following assert is true in all cases except when
    ** the database file has been corrupted externally.
    **    assert( u.an.zRec!=0 || u.an.avail>=u.an.payloadSize || u.an.avail>=9 ); */
    u.an.szHdr = getVarint32((byte*)u.an.zData, u.an.offset);

    /* Make sure a corrupt database has not given us an oversize header.
    ** Do this now to avoid an oversize memory allocation.
    **
    ** Type entries can be between 1 and 5 bytes each.  But 4 and 5 byte
    ** types use so much data space that there can only be 4096 and 32 of
    ** them, respectively.  So the maximum header length results from a
    ** 3-byte type for each of the maximum of 32768 columns plus three
    ** extra bytes for the header length itself.  32768*3 + 3 = 98307.
    */
    if( u.an.offset > 98307 ){
      rc = SQLITE_CORRUPT_BKPT;
      goto op_column_out;
    }

    /* Compute in u.an.len the number of bytes of data we need to read in order
    ** to get u.an.nField type values.  u.an.offset is an upper bound on this.  But
    ** u.an.nField might be significantly less than the true number of columns
    ** in the table, and in that case, 5*u.an.nField+3 might be smaller than u.an.offset.
    ** We want to minimize u.an.len in order to limit the size of the memory
    ** allocation, especially if a corrupt database file has caused u.an.offset
    ** to be oversized. Offset is limited to 98307 above.  But 98307 might
    ** still exceed Robson memory allocation limits on some configurations.
    ** On systems that cannot tolerate large memory allocations, u.an.nField*5+3
    ** will likely be much smaller since u.an.nField will likely be less than
    ** 20 or so.  This insures that Robson memory allocation limits are
    ** not exceeded even for corrupt database files.
    */
    u.an.len = u.an.nField*5 + 3;
    if( u.an.len > (int)u.an.offset ) u.an.len = (int)u.an.offset;

    /* The KeyFetch() or DataFetch() above are fast and will get the entire
    ** record header in most cases.  But they will fail to get the complete
    ** record header if the record header does not fit on a single page
    ** in the B-Tree.  When that happens, use sqlite3VdbeMemFromBtree() to
    ** acquire the complete header text.
    */
    if( !u.an.zRec && u.an.avail<u.an.len ){
      u.an.sMem.flags = 0;
      u.an.sMem.db = 0;
      rc = sqlite3VdbeMemFromBtree(u.an.pCrsr, 0, u.an.len, u.an.pC.isIndex, &u.an.sMem);
      if( rc!=SQLITE_OK ){
        goto op_column_out;
      }
      u.an.zData = u.an.sMem.z;
    }
    u.an.zEndHdr = (byte *)&u.an.zData[u.an.len];
    u.an.zIdx = (byte *)&u.an.zData[u.an.szHdr];

    /* Scan the header and use it to fill in the u.an.aType[] and u.an.aOffset[]
    ** arrays.  u.an.aType[u.an.i] will contain the type integer for the u.an.i-th
    ** column and u.an.aOffset[u.an.i] will contain the u.an.offset from the beginning
    ** of the record to the start of the data for the u.an.i-th column
    */
	count := 0
    for u.an.i = 0; u.an.i<u.an.nField; u.an.i++){
      if( u.an.zIdx<u.an.zEndHdr ){
        u.an.aOffset[u.an.i] = u.an.offset;
        if( u.an.zIdx[0]<0x80 ){
          u.an.t = u.an.zIdx[0];
          u.an.zIdx++;
        }else{
		  u.an.t, u.an.zIdx = u.an.zIdx.GetVarint32()
        }
        u.an.aType[u.an.i] = u.an.t;
        u.an.szField = VdbeSerialTypeLen(u.an.t)
        u.an.offset += u.an.szField;
        if( u.an.offset<u.an.szField ){  /* True if u.an.offset overflows */
          u.an.zIdx = &u.an.zEndHdr[1];  /* Forces SQLITE_CORRUPT return below */
          break;
        }
      }else{
        /* If u.an.i is less that u.an.nField, then there are fewer fields in this
        ** record than SetNumColumns indicated there are columns in the
        ** table. Set the u.an.offset for any extra columns not present in
        ** the record to 0. This tells code below to store the default value
        ** for the column instead of deserializing a value from the record.
        */
        u.an.aOffset[u.an.i] = 0;
      }
    }
    u.an.sMem.Release()
    u.an.sMem.Value = nil

    /* If we have read more header data than was contained in the header,
    ** or if the end of the last field appears to be past the end of the
    ** record, or if the end of the last field appears to be before the end
    ** of the record (when all fields present), then we must be dealing
    ** with a corrupt database.
    */
    if( (u.an.zIdx > u.an.zEndHdr) || (u.an.offset > u.an.payloadSize)
         || (u.an.zIdx==u.an.zEndHdr && u.an.offset!=u.an.payloadSize) ){
      rc = SQLITE_CORRUPT_BKPT;
      goto op_column_out;
    }
  }

  /* Get the column information. If u.an.aOffset[u.an.p2] is non-zero, then
  ** deserialize the value from the record. If u.an.aOffset[u.an.p2] is zero,
  ** then there are not enough fields in the record to satisfy the
  ** request.  In this case, set the value NULL or to P4 if P4 is
  ** a pointer to a Mem object.
  */
  if( u.an.aOffset[u.an.p2] ){
    assert( rc==SQLITE_OK );
    if( u.an.zRec ){
      /* This is the common case where the whole row fits on a single page */
      VdbeMemRelease(u.an.pDest);
      u.am.pDest.VdbeSerialGet(([]byte)(u.an.zRec[u.an.aOffset[u.an.p2]:]), u.an.aType[u.an.p2])
    }else{
      /* This branch happens only when the row overflows onto multiple pages */
      u.an.t = u.an.aType[u.an.p2];
      if( (pOp.p5 & (OPFLAG_LENGTHARG|OPFLAG_TYPEOFARG))!=0
       && ((u.an.t>=12 && (u.an.t&1)==0) || (pOp.p5 & OPFLAG_TYPEOFARG)!=0)
      ){
        /* Content is irrelevant for the typeof() function and for
        ** the length(X) function if X is a blob.  So we might as well use
        ** bogus content rather than reading content from disk.  NULL works
        ** for text and blob and whatever is in the u.an.payloadSize64 variable
        ** will work for everything else. */
        u.an.zData = u.an.t<12 ? (char*)&u.an.payloadSize64 : 0;
      }else{
        u.an.len = VdbeSerialTypeLen(u.an.t)
        sqlite3VdbeMemMove(&u.an.sMem, u.an.pDest);
        if rc = sqlite3VdbeMemFromBtree(u.an.pCrsr, u.an.aOffset[u.an.p2], u.an.len,  u.an.pC.isIndex, &u.an.sMem); rc != SQLITE_OK {
          goto op_column_out;
        }
        u.an.zData = u.an.sMem.z;
      }
      u.an.pDest.VdbeSerialGet(([]byte)(u.an.zData), u.an.t)
    }
    u.an.pDest.enc = encoding;
  }else{
    if( pOp.p4type==P4_MEM ){
      sqlite3VdbeMemShallowCopy(u.an.pDest, pOp.p4.pMem, MEM_Static);
    }else{
      u.an.pDest.Value = nil
    }
  }

  /* If we dynamically allocated space to hold the data (in the
  ** sqlite3VdbeMemFromBtree() call above) then transfer control of that
  ** dynamically allocated space over to the u.an.pDest structure.
  ** This prevents a memory copy.
  */
  if( u.an.sMem.zMalloc ){
    assert( u.an.sMem.z==u.an.sMem.zMalloc );
    assert( !(u.an.pDest.flags & MEM_Dyn) );
    assert( !(u.an.pDest.flags & (MEM_Blob|MEM_Str)) || u.an.pDest.z==u.an.sMem.z );
    u.an.pDest.flags &= ~(MEM_Ephem|MEM_Static);
    u.an.pDest.flags |= MEM_Term;
    u.an.pDest.z = u.an.sMem.z;
    u.an.pDest.zMalloc = u.an.sMem.zMalloc;
  }

  rc = sqlite3VdbeMemMakeWriteable(u.an.pDest);

op_column_out:
  UPDATE_MAX_BLOBSIZE(u.an.pDest);
  REGISTER_TRACE(pOp.p3, u.an.pDest);
  break;
}

/* Opcode: Affinity P1 P2 * P4 *
**
** Apply affinities to a range of P2 registers starting with P1.
**
** P4 is a string that is P2 characters long. The nth character of the
** string indicates the column affinity that should be used for the nth
** memory cell in the range.
*/
case OP_Affinity: {
  u.ao.zAffinity = pOp.p4.z;
  assert( u.ao.zAffinity!=0 );
  assert( u.ao.zAffinity[pOp.p2]==0 );
  pIn1 = &aMem[pOp.p1];
  while( (u.ao.cAff = *(u.ao.zAffinity++))!=0 ){
    assert( pIn1 <= &p.aMem[p.nMem] );
    assert( memIsValid(pIn1) );
    ExpandBlob(pIn1);
    applyAffinity(pIn1, u.ao.cAff, encoding);
    pIn1++;
  }
  break;
}

/* Opcode: MakeRecord P1 P2 P3 P4 *
**
** Convert P2 registers beginning with P1 into the [record format]
** use as a data record in a database table or as a key
** in an index.  The OP_Column opcode can decode the record later.
**
** P4 may be a string that is P2 characters long.  The nth character of the
** string indicates the column affinity that should be used for the nth
** field of the index key.
**
** The mapping from character to affinity is given by the SQLITE_AFF_
** macros defined in sqliteInt.h.
**
** If P4 is NULL then all index fields have the affinity NONE.
*/
case OP_MakeRecord: {
  /* Assuming the record contains N fields, the record format looks
  ** like this:
  **
  ** ------------------------------------------------------------------------
  ** | hdr-size | type 0 | type 1 | ... | type N-1 | data0 | ... | data N-1 |
  ** ------------------------------------------------------------------------
  **
  ** Data(0) is taken from register P1.  Data(1) comes from register P1+1
  ** and so froth.
  **
  ** Each type field is a varint representing the serial type of the
  ** corresponding data element (see VdbeSerialType()). The
  ** hdr-size field is also a varint which is the offset from the beginning
  ** of the record to data0.
  */
  u.ap.nData = 0;         /* Number of bytes of data space */
  u.ap.nHdr = 0;          /* Number of bytes of header space */
  u.ap.nZero = 0;         /* Number of zero bytes at the end of the record */
  u.ap.nField = pOp.p1;
  u.ap.zAffinity = pOp.p4.z;
  assert( u.ap.nField>0 && pOp.p2>0 && pOp.p2+u.ap.nField<=p.nMem+1 );
  u.ap.pData0 = &aMem[u.ap.nField];
  u.ap.nField = pOp.p2;
  u.ap.Last = &u.ap.pData0[u.ap.nField-1];
  u.ap.file_format = p.minWriteFileFormat;

  /* Identify the output register */
  assert( pOp.p3<pOp.p1 || pOp.p3>=pOp.p1+pOp.p2 );
  pOut = &aMem[pOp.p3];
  memAboutToChange(p, pOut);

	//	Loop through the elements that will make up the record to figure out how much space is required for the new record.
	for u.ap.pRec = u.ap.pData0; u.ap.pRec <= u.ap.Last; u.ap.pRec++ {
		assert( memIsValid(u.ap.pRec) )
		if( u.ap.zAffinity ){
			applyAffinity(u.ap.pRec, u.ap.zAffinity[u.ap.pRec-u.ap.pData0], encoding)
		}

		if v, ok := u.ap.pRec.Value.(Zeroes); ok && len(u.ap.pRec.z) > 0 {
			u.ap.pRec.ExpandBlob()
		}
		u.ap.serial_type = u.ap.pRec.VdbeSerialType(u.ap.file_format)
		u.ap.len = VdbeSerialTypeLen(u.ap.serial_type)
		u.ap.nData += u.ap.len;
		u.ap.nHdr += sqlite3VarintLen(u.ap.serial_type)
		if v, ok := u.ap.pRec.(Zeroes); ok {
			//	Only pure zero-filled BLOBs can be input to this Opcode. We do not allow blobs with a prefix and a zero-filled tail.
			u.ap.nZero += int(v)
		} else if u.ap.len != 0 {
			u.ap.nZero = 0
		}
	}

  /* Add the initial header varint and total the size */
  u.ap.nHdr += u.ap.nVarint = sqlite3VarintLen(u.ap.nHdr);
  if( u.ap.nVarint<sqlite3VarintLen(u.ap.nHdr) ){
    u.ap.nHdr++;
  }
  if u.ap.nByte = u.ap.nHdr + u.ap.nData - u.ap.nZero; u.ap.nByte < 0 {
    goto too_big;
  }

  /* Make sure the output register has a buffer large enough to store
  ** the new record. The output register (pOp.p3) is not allowed to
  ** be one of the input registers (because the following call to
  ** sqlite3VdbeMemGrow() could clobber the value before it is used).
  */
  if( sqlite3VdbeMemGrow(pOut, (int)u.ap.nByte, 0) ){
    goto no_mem;
  }
  u.ap.zNewRecord = (byte *)pOut.z;

  /* Write the record */
  u.ap.i = putVarint32(u.ap.zNewRecord, u.ap.nHdr);
  for(u.ap.pRec=u.ap.pData0; u.ap.pRec<=u.ap.Last; u.ap.pRec++){
    u.ap.serial_type = u.ap.pRec.VdbeSerialType(u.ap.file_format)
    u.ap.i += putVarint32(&u.ap.zNewRecord[u.ap.i], u.ap.serial_type);      /* serial type */
  }
  for(u.ap.pRec=u.ap.pData0; u.ap.pRec<=u.ap.Last; u.ap.pRec++){  /* serial data */
    u.ap.i += u.ap.pRec.VdbeSerialPut(&u.ap.zNewRecord[u.ap.i], u.ap.file_format)
  }
  assert( u.ap.i==u.ap.nByte );

  assert( pOp.p3>0 && pOp.p3<=p.nMem );
  pOut.n = (int)u.ap.nByte;
  pOut.flags = MEM_Blob | MEM_Dyn;
  pOut.xDel = 0;
  if u.ap.nZero != 0 {
    pOut.Value = Zeroes(u.ap.nZero)
  }
  pOut.enc = SQLITE_UTF8;  /* In case the blob is ever converted to text */
  REGISTER_TRACE(pOp.p3, pOut);
  UPDATE_MAX_BLOBSIZE(pOut);
  break;
}

/* Opcode: Count P1 P2 * * *
**
** Store the number of entries (an integer value) in the table or index 
** opened by cursor P1 in register P2
*/
case OP_Count: {         /* out2-prerelease */
  u.aq.pCrsr = p.apCsr[pOp.p1].pCursor;
  if( u.aq.pCrsr ){
    rc = sqlite3BtreeCount(u.aq.pCrsr, &u.aq.nEntry);
  }else{
    u.aq.nEntry = 0;
  }
  pOut.Store(u.aq.nEntry)
  break;
}

//	Opcode: Savepoint P1 * * P4 *
//	Open, release or rollback the savepoint named by parameter P4, depending on the value of P1. To open a new savepoint, P1==0. To release (commit) an existing savepoint, P1==1, or to rollback an existing savepoint P1==2.
case OP_Savepoint: {
	P1 := pOp.p1
	Name := pOp.p4.z

	//	Assert that the P1 parameter is valid. Also that if there is no open transaction, then there cannot be any savepoints.
	assert( db.Savepoints == nil || !db.autoCommit )
	assert( P1 == SAVEPOINT_BEGIN || P1 == SAVEPOINT_RELEASE || P1 == SAVEPOINT_ROLLBACK )
	assert( db.pSavepoint || !db.isTransactionSavepoint )

	switch {
	case P1 == SAVEPOINT_BEGIN:
		if db.writeVdbeCnt > 0 {
			//	A new savepoint cannot be created if there are active write statements (i.e. open read/write incremental blob handles).
			p.zErrMsg = "cannot open savepoint - SQL statements in progress"
			rc = SQLITE_BUSY
		} else {
			//	This call is Ok even if this savepoint is actually a transaction savepoint (and therefore should not prompt xSavepoint()) callbacks.
			//	If this is a transaction savepoint being opened, it is guaranteed that the db.aVTrans[] array is empty.
			assert( !db.autoCommit || db.nVTrans == 0 )
			rc = sqlite3VtabSavepoint(db, SAVEPOINT_BEGIN, db.nStatement + db.nSavepoint)
			if rc != SQLITE_OK {
				goto abort_due_to_error
			}

			//	If there is no open transaction, then mark this as a special "transaction savepoint".
			if db.autoCommit {
				db.autoCommit = false
				db.isTransactionSavepoint = true
				db.Savepoints = append(db.Savepoints, &Savepoint{ Name: Name, nDeferredCons: db.nDeferredCons, isTransaction: true })
			} else {
				db.Savepoints = append(db.Savepoints, &Savepoint{ Name: Name, nDeferredCons: db.nDeferredCons })
			}
		}
	case P1 == SAVEPOINT_RELEASE && db.writeVdbeCnt > 0:
		//	It is not possible to release (commit) a savepoint if there are active write statements.
		p.zErrMsg = "cannot release savepoint - SQL statements in progress"
		rc = SQLITE_BUSY
	} else {
		pSavepoint		*Savepoint
		iSavepoint := 0

		//	Find the named savepoint. If there is no such savepoint, then an error is returned to the user.
		for i := len(db.Savepoints) - 1; i > -1; i-- {
			pSavepoint = db.Savepoints[i]
			if !CaseInsensitiveMatch(pSavepoint.Name, u.ar.Name) {
				iSavepoint++
			} else {
				break
			}
		}
		if pSavepoint == nil {
			p.zErrMsg = fmt.Sprintf("no such savepoint: %v", u.ar.Name)
			rc = SQLITE_ERROR
		} else {
			//	Determine whether or not this is a transaction savepoint. If so, and this is a RELEASE command, then the current transaction is committed.
			isTransaction := pSavepoint.isTransaction && db.isTransactionSavepoint
			if isTransaction && P1 == SAVEPOINT_RELEASE {
				if rc = p.CheckFk(1); rc != SQLITE_OK {
					goto vdbe_return
				}
				db.autoCommit = true
				if p.Halt() == SQLITE_BUSY {
					p.pc = pc
					db.autoCommit = false
					p.rc = rc = SQLITE_BUSY
					goto vdbe_return
				}
				db.isTransactionSavepoint = false
				rc = p.rc
			} else {
				iSavepoint = len(db.nSavepoints) -1 - iSavepoint
				if P1 == SAVEPOINT_ROLLBACK {
					for _, database := range db.Databases {
						sqlite3BtreeTripAllCursors(database.pBt, SQLITE_ABORT)
					}
				}
				for _, database := range db.Databases {
					if rc = database.pBt.BtreeSavepoint(u.ar.p1, u.ar.SavepointStackDepth); rc != SQLITE_OK {
						goto abort_due_to_error
					}
				}
				if P1 == SAVEPOINT_ROLLBACK && (db.flags & SQLITE_InternChanges) != 0 {
					db.ExpirePreparedStatements()
					db.ResetInternalSchema(-1)
					db.flags = (db.flags | SQLITE_InternChanges)
				}
			}

			//	Regardless of whether this is a RELEASE or ROLLBACK, destroy all savepoints nested inside of the savepoint being operated on.
			while db.pSavepoint != u.ar.pSavepoint {
				u.ar.pTmp = db.pSavepoint
				db.pSavepoint = u.ar.pTmp.Next
				u.ar.pTmp = nil
				db.nSavepoint--
			}

			//	If it is a RELEASE, then destroy the savepoint being operated on too. If it is a ROLLBACK TO, then set the number of deferred constraint violations present in the database to the value stored when the savepoint was created.
			if P1 == SAVEPOINT_RELEASE {
				assert( u.ar.pSavepoint == db.pSavepoint )
				db.pSavepoint = u.ar.pSavepoint.Next
				u.ar.pSavepoint = nil
				if !isTransaction {
					db.nSavepoint--
				}
			} else {
				db.nDeferredCons = u.ar.pSavepoint.nDeferredCons;
			}

			if !isTransaction {
				if rc = sqlite3VtabSavepoint(db, u.ar.p1, u.ar.SavepointStackDepth); rc != SQLITE_OK {
					goto abort_due_to_error
				}
			}
		}
	}
	break
}

/* Opcode: AutoCommit P1 P2 * * *
**
** Set the database auto-commit flag to P1 (1 or 0). If P2 is true, roll
** back any currently active btree transactions. If there are any active
** VMs (apart from this one), then a ROLLBACK fails.  A COMMIT fails if
** there are active writing VMs or active VMs that use shared cache.
**
** This instruction causes the VM to halt.
*/
case OP_AutoCommit: {
  u.as.desiredAutoCommit = pOp.p1;
  u.as.iRollback = pOp.p2;
  u.as.turnOnAC = u.as.desiredAutoCommit && !db.autoCommit;
  assert( u.as.desiredAutoCommit==1 || u.as.desiredAutoCommit==0 );
  assert( u.as.desiredAutoCommit==1 || u.as.iRollback==0 );
  assert( db.activeVdbeCnt>0 );  /* At least this one VM is active */

  if( u.as.turnOnAC && !u.as.iRollback && db.writeVdbeCnt>0 ){
    /* If this instruction implements a COMMIT and other VMs are writing
    ** return an error indicating that the other VMs must complete first.
    */
    p.zErrMsg = "cannot commit transaction - SQL statements in progress"
    rc = SQLITE_BUSY;
  }else if( u.as.desiredAutoCommit!=db.autoCommit ){
    if( u.as.iRollback ){
      assert( u.as.desiredAutoCommit==1 );
      db.RollbackAll(SQLITE_ABORT_ROLLBACK)
      db.autoCommit = 1;
    }else if rc = p.CheckFk(1); rc != SQLITE_OK {
      goto vdbe_return;
    }else{
      db.autoCommit = (byte)u.as.desiredAutoCommit;
      if p.Halt() == SQLITE_BUSY {
        p.pc = pc;
        db.autoCommit = (byte)(1-u.as.desiredAutoCommit);
        p.rc = rc = SQLITE_BUSY;
        goto vdbe_return;
      }
    }
    assert( db.nStatement==0 );
    db.CloseSavepoints()
    if( p.rc==SQLITE_OK ){
      rc = SQLITE_DONE;
    }else{
      rc = SQLITE_ERROR;
    }
    goto vdbe_return;
  }else{
    p.zErrMsg = (!u.as.desiredAutoCommit) ? "cannot start a transaction within a transaction" : ((u.as.iRollback) ? "cannot rollback - no transaction is active" : "cannot commit - no transaction is active")
    rc = SQLITE_ERROR;
  }
  break;
}

/* Opcode: Transaction P1 P2 * * *
**
** Begin a transaction.  The transaction ends when a Commit or Rollback
** opcode is encountered.  Depending on the ON CONFLICT setting, the
** transaction might also be rolled back if an error is encountered.
**
** P1 is the index of the database file on which the transaction is
** started.  Index 0 is the main database file and index 1 is the
** file used for temporary tables.  Indices of 2 or more are used for
** attached databases.
**
** If P2 is non-zero, then a write-transaction is started.  A RESERVED lock is
** obtained on the database file when a write-transaction is started.  No
** other process can start another write transaction while this transaction is
** underway.  Starting a write transaction also creates a rollback journal. A
** write transaction must be started before any changes can be made to the
** database.  If P2 is 2 or greater then an EXCLUSIVE lock is also obtained
** on the file.
**
** If a write-transaction is started and the Vdbe.usesStmtJournal flag is
** true (this flag is set if the Vdbe may modify more than one row and may
** throw an ABORT exception), a statement transaction may also be opened.
** More specifically, a statement transaction is opened iff the database
** connection is currently not in autocommit mode, or if there are other
** active statements. A statement transaction allows the changes made by this
** VDBE to be rolled back after an error without having to roll back the
** entire transaction. If no error is encountered, the statement transaction
** will automatically commit when the VDBE halts.
**
** If P2 is zero, then a read-lock is obtained on the database file.
*/
case OP_Transaction: {
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<pOp.p1))!=0 );
  u.at.pBt = db.Databases[pOp.p1].pBt;

  if( u.at.pBt ){
    rc = sqlite3BtreeBeginTrans(u.at.pBt, pOp.p2);
    if( rc==SQLITE_BUSY ){
      p.pc = pc;
      p.rc = rc = SQLITE_BUSY;
      goto vdbe_return;
    }
    if( rc!=SQLITE_OK ){
      goto abort_due_to_error;
    }

    if( pOp.p2 && p.usesStmtJournal
     && (db.autoCommit==0 || db.activeVdbeCnt>1)
    ){
      assert( u.at.pBt.IsInTrans() )
      if( p.iStatement==0 ){
        assert( db.nStatement>=0 && db.nSavepoint>=0 );
        db.nStatement++;
        p.iStatement = db.nSavepoint + db.nStatement;
      }

      rc = sqlite3VtabSavepoint(db, SAVEPOINT_BEGIN, p.iStatement-1);
      if( rc==SQLITE_OK ){
        rc = sqlite3BtreeBeginStmt(u.at.pBt, p.iStatement);
      }

      /* Store the current value of the database handles deferred constraint
      ** counter. If the statement transaction needs to be rolled back,
      ** the value of this counter needs to be restored too.  */
      p.nStmtDefCons = db.nDeferredCons;
    }
  }
  break;
}

/* Opcode: ReadCookie P1 P2 P3 * *
**
** Read cookie number P3 from database P1 and write it into register P2.
** P3==1 is the schema version.  P3==2 is the database format.
** P3==3 is the recommended pager cache size, and so forth.  P1==0 is
** the main database file and P1==1 is the database file used to store
** temporary tables.
**
** There must be a read-lock on the database (either a transaction
** must be started or there must be an open cursor) before
** executing this instruction.
*/
case OP_ReadCookie: {               /* out2-prerelease */
  u.au.iDb = pOp.p1;
  u.au.iCookie = pOp.p3;
  assert( pOp.p3<SQLITE_N_BTREE_META );
  assert( u.au.iDb >= 0 && u.au.iDb < len(db.Databases) )
  assert( db.Databases[u.au.iDb].pBt!=0 );
  assert( (p.btreeMask & (((yDbMask)1)<<u.au.iDb))!=0 );

  sqlite3BtreeGetMeta(db.Databases[u.au.iDb].pBt, u.au.iCookie, (uint32 *)&u.au.iMeta);
  pOut.Store(u.au.iMeta)
  break;
}

/* Opcode: SetCookie P1 P2 P3 * *
**
** Write the content of register P3 (interpreted as an integer)
** into cookie number P2 of database P1.  P2==1 is the schema version.  
** P2==2 is the database format. P2==3 is the recommended pager cache 
** size, and so forth.  P1==0 is the main database file and P1==1 is the 
** database file used to store temporary tables.
**
** A transaction must be started before executing this opcode.
*/
case OP_SetCookie: {       /* in3 */
  assert( pOp.p2<SQLITE_N_BTREE_META );
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<pOp.p1))!=0 );
  u.av.pDb = &db.Databases[pOp.p1];
  assert( u.av.pDb.pBt!=0 );
  pIn3 = &aMem[pOp.p3];
  pIn3.Integerify()
  /* See note about index shifting on OP_ReadCookie */
  rc = sqlite3BtreeUpdateMeta(u.av.pDb.pBt, pOp.p2, int(pIn3.Integer()))
  if( pOp.p2==BTREE_SCHEMA_VERSION ){
    /* When the schema cookie changes, record the new cookie internally */
    u.av.pDb.Schema.schema_cookie = int(pIn3.Integer())
    db.flags |= SQLITE_InternChanges;
  }else if( pOp.p2==BTREE_FILE_FORMAT ){
    /* Record changes in the file format */
    u.av.pDb.Schema.file_format = byte(pIn3.Integer())
  }
  if( pOp.p1==1 ){
    /* Invalidate all prepared statements whenever the TEMP database
    ** schema is changed.  Ticket #1644 */
    db.ExpirePreparedStatements()
    p.expired = 0;
  }
  break;
}

/* Opcode: VerifyCookie P1 P2 P3 * *
**
** Check the value of global database parameter number 0 (the
** schema version) and make sure it is equal to P2 and that the
** generation counter on the local schema parse equals P3.
**
** P1 is the database number which is 0 for the main database file
** and 1 for the file holding temporary tables and some higher number
** for auxiliary databases.
**
** The cookie changes its value whenever the database schema changes.
** This operation is used to detect when that the cookie has changed
** and that the current process needs to reread the schema.
**
** Either a transaction needs to have been started or an OP_Open needs
** to be executed (to establish a read lock) before this opcode is
** invoked.
*/
case OP_VerifyCookie: {
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<pOp.p1))!=0 );
  u.aw.pBt = db.Databases[pOp.p1].pBt;
  if( u.aw.pBt ){
    sqlite3BtreeGetMeta(u.aw.pBt, BTREE_SCHEMA_VERSION, (uint32 *)&u.aw.iMeta);
    u.aw.iGen = db.Databases[pOp.p1].Schema.iGeneration;
  }else{
    u.aw.iGen = u.aw.iMeta = 0;
  }
  if( u.aw.iMeta!=pOp.p2 || u.aw.iGen!=pOp.p3 ){
    p.zErrMsg = sqlite3DbStrDup(db, "database schema has changed");
    /* If the schema-cookie from the database file matches the cookie
    ** stored with the in-memory representation of the schema, do
    ** not reload the schema from the database file.
    **
    ** If virtual-tables are in use, this is not just an optimization.
    ** Often, v-tables store their data in other SQLite tables, which
    ** are queried from within xNext() and other v-table methods using
    ** prepared queries. If such a query is out-of-date, we do not want to
    ** discard the database schema, as the user code implementing the
    ** v-table would have to be ready for the sqlite3_vtab structure itself
    ** to be invalidated whenever sqlite3_step() is called from within
    ** a v-table method.
    */
    if( db.Databases[pOp.p1].Schema.schema_cookie!=u.aw.iMeta ){
      db.ResetInternalSchema(pOp.p1)
    }

    p.expired = 1;
    rc = SQLITE_SCHEMA;
  }
  break;
}

/* Opcode: OpenRead P1 P2 P3 P4 P5
**
** Open a read-only cursor for the database table whose root page is
** P2 in a database file.  The database file is determined by P3. 
** P3==0 means the main database, P3==1 means the database used for 
** temporary tables, and P3>1 means used the corresponding attached
** database.  Give the new cursor an identifier of P1.  The P1
** values need not be contiguous but all P1 values should be small integers.
** It is an error for P1 to be negative.
**
** If P5!=0 then use the content of register P2 as the root page, not
** the value of P2 itself.
**
** There will be a read lock on the database whenever there is an
** open cursor.  If the database was unlocked prior to this instruction
** then a read lock is acquired as part of this instruction.  A read
** lock allows other processes to read the database but prohibits
** any other process from modifying the database.  The read lock is
** released when all cursors are closed.  If this instruction attempts
** to get a read lock but fails, the script terminates with an
** SQLITE_BUSY error code.
**
** The P4 value may be either an integer (P4_INT32) or a pointer to
** a KeyInfo structure (P4_KEYINFO). If it is a pointer to a KeyInfo 
** structure, then said structure defines the content and collating 
** sequence of the index being opened. Otherwise, if P4 is an integer 
** value, it is set to the number of columns in the table.
**
** See also OpenWrite.
*/
/* Opcode: OpenWrite P1 P2 P3 P4 P5
**
** Open a read/write cursor named P1 on the table or index whose root
** page is P2.  Or if P5!=0 use the content of register P2 to find the
** root page.
**
** The P4 value may be either an integer (P4_INT32) or a pointer to
** a KeyInfo structure (P4_KEYINFO). If it is a pointer to a KeyInfo 
** structure, then said structure defines the content and collating 
** sequence of the index being opened. Otherwise, if P4 is an integer 
** value, it is set to the number of columns in the table, or to the
** largest index of any column of the table that is actually used.
**
** This instruction works just like OpenRead except that it opens the cursor
** in read/write mode.  For a given table, there can be one or more read-only
** cursors or a single read/write cursor but not both.
**
** See also OpenRead.
*/
case OP_OpenRead:
case OP_OpenWrite: {
  if( p.expired ){
    rc = SQLITE_ABORT;
    break;
  }

  u.ax.nField = 0;
  u.ax.pKeyInfo = 0;
  u.ax.p2 = pOp.p2;
  u.ax.iDb = pOp.p3;
  assert( u.ax.iDb >= 0 && u.ax.iDb < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<u.ax.iDb))!=0 );
  u.ax.pDb = &db.Databases[u.ax.iDb];
  u.ax.pX = u.ax.pDb.pBt;
  assert( u.ax.pX!=0 );
  if( pOp.opcode==OP_OpenWrite ){
    u.ax.Writable = true;
    if( u.ax.pDb.Schema.file_format < p.minWriteFileFormat ){
      p.minWriteFileFormat = u.ax.pDb.Schema.file_format;
    }
  }else{
    u.ax.Writable = false;
  }
  if( pOp.p5 ){
    assert( u.ax.p2>0 );
    assert( u.ax.p2<=p.nMem );
    pIn2 = &aMem[u.ax.p2];
    assert( memIsValid(pIn2) );
    assert( (pIn2.flags & )!=0 );
    pIn2.Integerify()
    u.ax.p2 = int(pIn2.Integer())
    /* The u.ax.p2 value always comes from a prior OP_CreateTable opcode and
    ** that opcode will always set the u.ax.p2 value to 2 or more or else fail.
    ** If there were a failure, the prepared statement would have halted
    ** before reaching this instruction. */
    if( u.ax.p2<2 ) {
      rc = SQLITE_CORRUPT_BKPT;
      goto abort_due_to_error;
    }
  }
  if( pOp.p4type==P4_KEYINFO ){
    u.ax.pKeyInfo = pOp.p4.pKeyInfo;
    u.ax.pKeyInfo.enc = p.db.Encoding()
    u.ax.nField = u.ax.pKeyInfo.nField+1;
  }else if( pOp.p4type==P4_INT32 ){
    u.ax.nField = pOp.p4.i;
  }
  assert( pOp.p1>=0 );
  u.ax.pCur = allocateCursor(p, pOp.p1, u.ax.nField, u.ax.iDb, 1);
  if( u.ax.pCur==0 ) goto no_mem;
  u.ax.pCur.nullRow = 1;
  u.ax.pCur.isOrdered = 1;
  rc = sqlite3BtreeCursor(u.ax.pX, u.ax.p2, u.ax.Writable, u.ax.pKeyInfo, u.ax.pCur.pCursor);
  u.ax.pCur.pKeyInfo = u.ax.pKeyInfo;

  /* Since it performs no memory allocation or IO, the only value that
  ** sqlite3BtreeCursor() may return is SQLITE_OK. */
  assert( rc==SQLITE_OK );

  /* Set the VdbeCursor.isTable and isIndex variables. Previous versions of
  ** SQLite used to check if the root-page flags were sane at this point
  ** and report database corruption if they were not, but this check has
  ** since moved into the btree layer.  */
  u.ax.pCur.isTable = pOp.p4type!=P4_KEYINFO;
  u.ax.pCur.isIndex = !u.ax.pCur.isTable;
  break;
}

/* Opcode: OpenEphemeral P1 P2 * P4 P5
**
** Open a new cursor P1 to a transient table.
** The cursor is always opened read/write even if 
** the main database is read-only.  The ephemeral
** table is deleted automatically when the cursor is closed.
**
** P2 is the number of columns in the ephemeral table.
** The cursor points to a BTree table if P4==0 and to a BTree index
** if P4 is not 0.  If P4 is not NULL, it points to a KeyInfo structure
** that defines the format of keys in the index.
**
** This opcode was once called OpenTemp.  But that created
** confusion because the term "temp table", might refer either
** to a TEMP table at the SQL level, or to a table opened by
** this opcode.  Then this opcode was call OpenVirtual.  But
** that created confusion with the whole virtual-table idea.
**
** The P5 parameter can be a mask of the BTREE_* flags defined
** in btree.h.  These flags control aspects of the operation of
** the btree.  The BTREE_OMIT_JOURNAL and BTREE_SINGLE flags are
** added automatically.
*/
/* Opcode: OpenAutoindex P1 P2 * P4 *
**
** This opcode works the same as OP_OpenEphemeral.  It has a
** different name to distinguish its use.  Tables created using
** by this opcode will be used for automatically created transient
** indices in joins.
*/
case OP_OpenAutoindex: 
case OP_OpenEphemeral: {
  static const int vfsFlags =
      SQLITE_OPEN_READWRITE |
      SQLITE_OPEN_CREATE |
      SQLITE_OPEN_EXCLUSIVE |
      SQLITE_OPEN_DELETEONCLOSE |
      SQLITE_OPEN_TRANSIENT_DB;

  assert( pOp.p1>=0 );
  u.ay.pCx = allocateCursor(p, pOp.p1, pOp.p2, -1, 1);
  if( u.ay.pCx==0 ) goto no_mem;
  u.ay.pCx.nullRow = 1;
  rc = sqlite3BtreeOpen(db.pVfs, 0, db, &u.ay.pCx.pBt,
                        BTREE_OMIT_JOURNAL | BTREE_SINGLE | pOp.p5, vfsFlags);
  if( rc==SQLITE_OK ){
    rc = sqlite3BtreeBeginTrans(u.ay.pCx.pBt, 1);
  }
  if( rc==SQLITE_OK ){
    /* If a transient index is required, create it by calling
    ** sqlite3BtreeCreateTable() with the BTREE_BLOBKEY flag before
    ** opening it. If a transient table is required, just use the
    ** automatically created table with root-page 1 (an BLOB_INTKEY table).
    */
    if( pOp.p4.pKeyInfo ){
      int pgno;
      assert( pOp.p4type==P4_KEYINFO );
      rc = sqlite3BtreeCreateTable(u.ay.pCx.pBt, &pgno, BTREE_BLOBKEY | pOp.p5);
      if( rc==SQLITE_OK ){
        assert( pgno==MASTER_ROOT+1 );
        rc = sqlite3BtreeCursor(u.ay.pCx.pBt, pgno, 1, (KeyInfo*)pOp.p4.z, u.ay.pCx.pCursor);
        u.ay.pCx.pKeyInfo = pOp.p4.pKeyInfo;
        u.ay.pCx.pKeyInfo.enc = p.db.Encoding()
      }
      u.ay.pCx.isTable = 0;
    }else{
      rc = sqlite3BtreeCursor(u.ay.pCx.pBt, MASTER_ROOT, 1, 0, u.ay.pCx.pCursor);
      u.ay.pCx.isTable = 1;
    }
  }
  u.ay.pCx.isOrdered = (pOp.p5!=BTREE_UNORDERED);
  u.ay.pCx.isIndex = !u.ay.pCx.isTable;
  break;
}

/* Opcode: OpenSorter P1 P2 * P4 *
**
** This opcode works like OP_OpenEphemeral except that it opens
** a transient index that is specifically designed to sort large
** tables using an external merge-sort algorithm.
*/
case OP_SorterOpen: {
  u.az.pCx = allocateCursor(p, pOp.p1, pOp.p2, -1, 1);
  if( u.az.pCx==0 ) goto no_mem;
  u.az.pCx.pKeyInfo = pOp.p4.pKeyInfo;
  u.az.pCx.pKeyInfo.enc = p.db.Encoding()
  u.az.pCx.isSorter = 1;
  rc = sqlite3VdbeSorterInit(db, u.az.pCx);
  break;
}

/* Opcode: OpenPseudo P1 P2 P3 * *
**
** Open a new cursor that points to a fake table that contains a single
** row of data.  The content of that one row in the content of memory
** register P2.  In other words, cursor P1 becomes an alias for the 
** MEM_Blob content contained in register P2.
**
** A pseudo-table created by this opcode is used to hold a single
** row output from the sorter so that the row can be decomposed into
** individual columns using the OP_Column opcode.  The OP_Column opcode
** is the only cursor opcode that works with a pseudo-table.
**
** P3 is the number of fields in the records that will be stored by
** the pseudo-table.
*/
case OP_OpenPseudo: {
  assert( pOp.p1>=0 );
  u.ba.pCx = allocateCursor(p, pOp.p1, pOp.p3, -1, 0);
  if( u.ba.pCx==0 ) goto no_mem;
  u.ba.pCx.nullRow = 1;
  u.ba.pCx.pseudoTableReg = pOp.p2;
  u.ba.pCx.isTable = 1;
  u.ba.pCx.isIndex = 0;
  break;
}

//	Opcode: Close P1 * * * *
//	Close a cursor previously opened as P1. If P1 is not currently open, this instruction is a no-op.
case OP_Close: {
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	p.FreeCursor(p.apCsr[pOp.p1])
	p.apCsr[pOp.p1] = nil
}

//	Opcode: SeekGe P1 P2 P3 P4 *
//	If cursor P1 refers to an SQL table (B-Tree that uses integer keys), use the value in register P3 as the key. If cursor P1 refers to an SQL index, then P3 is the first in an array of P4 registers that are used as an unpacked index key.
//	Reposition cursor P1 so that  it points to the smallest entry that is greater than or equal to the key value. If there are no records greater than or equal to the key and P2 is not zero, then jump to P2.
//	See also: Found, NotFound, Distinct, SeekLt, SeekGt, SeekLe
//
//	Opcode: SeekGt P1 P2 P3 P4 *
//	If cursor P1 refers to an SQL table (B-Tree that uses integer keys), use the value in register P3 as a key. If cursor P1 refers to an SQL index, then P3 is the first in an array of P4 registers that are used as an unpacked index key.
//	Reposition cursor P1 so that it points to the smallest entry that is greater than the key value. If there are no records greater than the key and P2 is not zero, then jump to P2.
//	See also: Found, NotFound, Distinct, SeekLt, SeekGe, SeekLe
//
//	Opcode: SeekLt P1 P2 P3 P4 * 
//	If cursor P1 refers to an SQL table (B-Tree that uses integer keys), use the value in register P3 as a key. If cursor P1 refers to an SQL index, then P3 is the first in an array of P4 registers that are used as an unpacked index key.
//	Reposition cursor P1 so that  it points to the largest entry that is less than the key value. If there are no records less than the key and P2 is not zero, then jump to P2.
//	See also: Found, NotFound, Distinct, SeekGt, SeekGe, SeekLe
//
//	Opcode: SeekLe P1 P2 P3 P4 *
//	If cursor P1 refers to an SQL table (B-Tree that uses integer keys), use the value in register P3 as a key. If cursor P1 refers to an SQL index, then P3 is the first in an array of P4 registers that are used as an unpacked index key.
//	Reposition cursor P1 so that it points to the largest entry that is less than or equal to the key value. If there are no records less than or equal to the key and P2 is not zero, then jump to P2.
//	See also: Found, NotFound, Distinct, SeekGt, SeekGe, SeekLt
case OP_SeekLt:		fallthrough		//	jump, in3
case OP_SeekLe:		fallthrough		//	jump, in3
case OP_SeekGe:		fallthrough		//	jump, in3
case OP_SeekGt:						//	jump, in3
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	assert( pOp.p2 != 0 )
	u.bb.pC = p.apCsr[pOp.p1]
	assert( u.bb.pC != nil )
	assert( u.bb.pC.pseudoTableReg == 0 )
	assert( OP_SeekLe == OP_SeekLt + 1 )
	assert( OP_SeekGe == OP_SeekLt + 2 )
	assert( OP_SeekGt == OP_SeekLt + 3 )
	assert( u.bb.pC.isOrdered )
	if u.bb.pC.pCursor != nil {
		u.bb.oc = pOp.opcode
		u.bb.pC.nullRow = 0
		if u.bb.pC.isTable {
			//	The input value in P3 might be of any type: integer, real, string, blob, or NULL.  But it needs to be an integer before we can do the seek, so covert it.
			pIn3 = &aMem[pOp.p3]
			pIn3.applyNumericAffinity()
			u.bb.iKey = pIn3.IntValue()
			u.bb.pC.rowidIsValid = 0

			//	If the P3 value could not be converted into an integer without loss of information, then special processing is required...
			if _, ok := pIn3.Value.(int64); !ok {
				if pIn3.flags & MEM_Real == 0 {
					//	If the P3 value cannot be converted into any kind of a number, then the seek is not possible, so jump to P2
					pc = pOp.p2 - 1
				}
				//	If we reach this point, then the P3 value must be a floating point number.
				assert( pIn3.flags & MEM_Real != 0 )
				if u.bb.iKey == SMALLEST_INT64 && (pIn3.r < float64(u.bb.iKey) || pIn3.r > 0) {
					//	The P3 value is too large in magnitude to be expressed as an integer.
					u.bb.res = 1
					if pIn3.r < 0 {
						if u.bb.oc >= OP_SeekGe {
							assert( u.bb.oc == OP_SeekGe || u.bb.oc == OP_SeekGt )
							if rc = sqlite3BtreeFirst(u.bb.pC.pCursor, &u.bb.res); rc != SQLITE_OK {
								goto abort_due_to_error
							}
						}
					} else {
 						if u.bb.oc <= OP_SeekLe {
							assert( u.bb.oc == OP_SeekLt || u.bb.oc == OP_SeekLe )
							if rc = sqlite3BtreeLast(u.bb.pC.pCursor, &u.bb.res); rc != SQLITE_OK {
								goto abort_due_to_error
							}
						}
					}
					if u.bb.res {
						pc = pOp.p2 - 1
					}
					break
				} else if u.bb.oc == OP_SeekLt || u.bb.oc == OP_SeekGe {
					//	Use the ceiling() function to convert real.int
					if pIn3.r > float64(u.bb.iKey) {
						u.bb.iKey++
					}
				} else {
					//	Use the floor() function to convert real.int
					assert( u.bb.oc == OP_SeekLe || u.bb.oc == OP_SeekGt )
					if pIn3.r < float64(u.bb.iKey) {
						u.bb.iKey--
					}
				}
			}
			if u.bb.res, rc = u.bb.pC.pCursor.BtreeMovetoUnpacked(0, uint64(u.bb.iKey), false); rc != SQLITE_OK {
				goto abort_due_to_error
			}
			if u.bb.res == 0 {
				u.bb.pC.rowidIsValid = 1
				u.bb.pC.lastRowid = u.bb.iKey
			}
		} else {
			u.bb.nField = pOp.p4.i
			assert( pOp.p4type == P4_INT32 )
			assert( u.bb.nField > 0 )
			u.bb.r.pKeyInfo = u.bb.pC.pKeyInfo
			u.bb.r.nField = uint16(u.bb.nField)

			//	The next line of code computes as follows, only faster:
			//		if u.bb.oc == OP_SeekGt || u.bb.oc == OP_SeekLe {
			//			u.bb.r.flags = UNPACKED_INCRKEY
			//		} else {
			//			u.bb.r.flags = 0
			//		}
			u.bb.r.flags = uint16(UNPACKED_INCRKEY * (1 & (u.bb.oc - OP_SeekLt)))
			assert( u.bb.oc != OP_SeekGt || u.bb.r.flags == UNPACKED_INCRKEY )
			assert( u.bb.oc != OP_SeekLe || u.bb.r.flags == UNPACKED_INCRKEY )
			assert( u.bb.oc != OP_SeekGe || u.bb.r.flags == 0 )
			assert( u.bb.oc != OP_SeekLt || u.bb.r.flags == 0 )

			u.bb.r.aMem = &aMem[pOp.p3]
			ExpandBlob(u.bb.r.aMem)
			if u.bb.res, rc = u.bb.pC.pCursor.BtreeMovetoUnpacked(&u.bb.r, 0, false); rc != SQLITE_OK {
				goto abort_due_to_error
			}
			u.bb.pC.rowidIsValid = 0
		}
		u.bb.pC.deferredMoveto = 0
		u.bb.pC.cacheStatus = CACHE_STALE
		if u.bb.oc >= OP_SeekGe {
			assert( u.bb.oc == OP_SeekGe || u.bb.oc == OP_SeekGt )
			if u.bb.res < 0 || (u.bb.res == 0 && u.bb.oc == OP_SeekGt) {
				if rc = sqlite3BtreeNext(u.bb.pC.pCursor, &u.bb.res); rc != SQLITE_OK {
					goto abort_due_to_error
				}
				u.bb.pC.rowidIsValid = 0
			} else {
				u.bb.res = 0
			}
		} else {
			assert( u.bb.oc == OP_SeekLt || u.bb.oc == OP_SeekLe )
			if u.bb.res > 0 || (u.bb.res == 0 && u.bb.oc == OP_SeekLt) {
				if rc = sqlite3BtreePrevious(u.bb.pC.pCursor, &u.bb.res); rc != SQLITE_OK {
					goto abort_due_to_error
				}
				u.bb.pC.rowidIsValid = 0
			} else {
				//	u.bb.res might be negative because the table is empty. Check to see if this is the case.
				u.bb.res = sqlite3BtreeEof(u.bb.pC.pCursor);
			}
		}
		assert( pOp.p2 > 0 )
		if u.bb.res {
			pc = pOp.p2 - 1
		}
	} else {
		//	This happens when attempting to open the sqlite3_master table for read access returns SQLITE_EMPTY. In this case always take the jump (since there are no records in the table).
		pc = pOp.p2 - 1
	}

//	Opcode: Seek P1 P2 * * *
//	P1 is an open table cursor and P2 is a rowid integer. Arrange for P1 to move so that it points to the rowid given by P2.
//	This is actually a deferred seek. Nothing actually happens until the cursor is used to read a record. That way, if no reads occur, no unnecessary I/O happens.
case OP_Seek:			//	in2
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bc.pC = p.apCsr[pOp.p1]
	assert( u.bc.pC != nil )
	if u.bc.pC.pCursor != nil {
		assert( u.bc.pC.isTable )
		u.bc.pC.nullRow = 0
		pIn2 = &aMem[pOp.p2]
		u.bc.pC.movetoTarget = pIn2.IntValue()
		u.bc.pC.rowidIsValid = 0
		u.bc.pC.deferredMoveto = 1
	}
  
//	Opcode: Found P1 P2 P3 P4 *
//	If P4 == 0 then register P3 holds a blob constructed by MakeRecord. If P4 > 0 then register P3 is the first of P4 registers that form an unpacked record.
//	Cursor P1 is on an index btree. If the record identified by P3 and P4 is a prefix of any entry in P1 then a jump is made to P2 and P1 is left pointing at the matching entry.
//
//	Opcode: NotFound P1 P2 P3 P4 *
//	If P4 == 0 then register P3 holds a blob constructed by MakeRecord. If P4 > 0 then register P3 is the first of P4 registers that form an unpacked record.
//	Cursor P1 is on an index btree. If the record identified by P3 and P4 is not the prefix of any entry in P1 then a jump is made to P2. If P1 does contain an entry whose prefix matches the P3/P4 record then control falls through to the next instruction and P1 is left pointing at the matching entry.
//	See also: Found, NotExists, IsUnique
case OP_NotFound:		fallthrough		//	jump, in3
case OP_Found:							//	jump, in3
	u.bd.alreadyExists = 0
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	assert( pOp.p4type == P4_INT32 )
	u.bd.pC = p.apCsr[pOp.p1]
	assert( u.bd.pC != nil )
	pIn3 = &aMem[pOp.p3]
	if u.bd.pC.pCursor != nil {
		assert( u.bd.pC.isTable == 0 )
		if pOp.p4.i > 0 {
			u.bd.r.pKeyInfo = u.bd.pC.pKeyInfo
			u.bd.r.nField = uint16(pOp.p4.i)
			u.bd.r.aMem = pIn3
			u.bd.r.flags = UNPACKED_PREFIX_MATCH
			u.bd.pIdxKey = &u.bd.r
		} else {
			u.bd.pIdxKey = sqlite3VdbeAllocUnpackedRecord(u.bd.pC.pKeyInfo, u.bd.aTempRec, sizeof(u.bd.aTempRec), &u.bd.pFree)
			if u.bd.pIdxKey == nil {
				goto no_mem
			}
			assert( pIn3.flags & MEM_Blob )
			sqlite3VdbeRecordUnpack(u.bd.pC.pKeyInfo, pIn3.n, pIn3.z, u.bd.pIdxKey)
			u.bd.pIdxKey.flags |= UNPACKED_PREFIX_MATCH
		}
		u.bd.res, rc = u.bd.pC.pCursor.BtreeMovetoUnpacked(u.bd.pIdxKey, 0, false)
		if pOp.p4.i == 0 {
			u.bd.pFree = nil
		}
		if rc == SQLITE_OK {
			u.bd.alreadyExists = u.bd.res == 0
			u.bd.pC.deferredMoveto = 0
			u.bd.pC.cacheStatus = CACHE_STALE
		}
		if pOp.opcode == OP_Found {
			if u.bd.alreadyExists {
				pc = pOp.p2 - 1
			}
		} else {
			if !u.bd.alreadyExists {
				pc = pOp.p2 - 1
			}
		}
	}

//	Opcode: IsUnique P1 P2 P3 P4 *
//	Cursor P1 is open on an index b-tree - that is to say, a btree which no data and where the key are records generated by OP_MakeRecord with the list field being the integer ROWID of the entry that the index entry refers to.
//	The P3 register contains an integer record number. Call this record  number R. Register P4 is the first in a set of N contiguous registers that make up an unpacked index key that can be used with cursor P1. The value of N can be inferred from the cursor. N includes the rowid value appended to the end of the index record. This rowid value may or may not be the same as R.
//	If any of the N registers beginning with register P4 contains a NULL value, jump immediately to P2.
//	Otherwise, this instruction checks if cursor P1 contains an entry where the first (N-1) fields match but the rowid value at the end of the index entry is not R. If there is no such entry, control jumps to instruction P2. Otherwise, the rowid of the conflicting index entry is copied to register P3 and control falls through to the next instruction.
//	See also: NotFound, NotExists, Found
case OP_IsUnique:				//	jump, in3
	pIn3 = &aMem[pOp.p3]
	u.be.aMx = &aMem[pOp.p4.i]
	//	Assert that the values of parameters P1 and P4 are in range.
	assert( pOp.p4type == P4_INT32 )
	assert( pOp.p4.i > 0 && pOp.p4.i <= p.nMem )
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )

	//	Find the index cursor.
	u.be.pCx = p.apCsr[pOp.p1]
	assert( u.be.pCx.deferredMoveto == 0 )
	u.be.pCx.seekResult = 0
	u.be.pCx.cacheStatus = CACHE_STALE
	u.be.pCrsr = u.be.pCx.pCursor

	//	If any of the values are NULL, take the jump.
	u.be.nField = u.be.pCx.pKeyInfo.nField
	for u.be.ii = 0; u.be.ii < u.be.nField; u.be.ii++ {
		if u.be.aMx[u.be.ii].Value == nil {
			pc = pOp.p2 - 1
			u.be.pCrsr = 0
			break
		}
	}
	assert( u.be.aMx[u.be.nField].Value != nil )

	if u.be.pCrsr != nil {
		//	Populate the index search key.
		u.be.r.pKeyInfo = u.be.pCx.pKeyInfo
		u.be.r.nField = u.be.nField + 1
		u.be.r.flags = UNPACKED_PREFIX_SEARCH
		u.be.r.aMem = u.be.aMx

		//	Extract the value of u.be.R from register P3.
		pIn3.Integerify()
		u.be.R = pIn3.Integer()

		//	Search the B-Tree index. If no conflicting record is found, jump to P2. Otherwise, copy the rowid of the conflicting record to register P3 and fall through to the next instruction.
		u.be.pCx.seekResult, rc = u.be.pCrsr.BtreeMovetoUnpacked(&u.be.r, 0, false)
		if (u.be.r.flags & UNPACKED_PREFIX_SEARCH) || u.be.r.rowid == u.be.R {
			pc = pOp.p2 - 1
		} else {
			pIn3.Store(u.be.r.rowid)
		}
	}

//	Opcode: NotExists P1 P2 P3 * *
//	Use the content of register P3 as an integer key. If a record with that key does not exist in table of P1, then jump to P2. If the record does exist, then fall through. The cursor is left pointing to the record if it exists.
//	The difference between this operation and NotFound is that this operation assumes the key is an integer and that P1 is a table whereas NotFound assumes key is a blob constructed from MakeRecord and P1 is an index.
//	See also: Found, NotFound, IsUnique
case OP_NotExists:				//	jump, in3
	pIn3 = &aMem[pOp.p3]
	assert( pIn3.Value.(int64)  )
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bf.pC = p.apCsr[pOp.p1]
	assert( u.bf.pC != nil )
	assert( u.bf.pC.isTable )
	assert( u.bf.pC.pseudoTableReg == 0 )
	if u.bf.pCrsr = u.bf.pC.pCursor; u.bf.pCrsr != nil {
		u.bf.res = 0
		u.bf.iKey = pIn3.Integer()
		u.bf.res, rc = u.bf.pCrsr.BtreeMovetoUnpacked(0, u.bf.iKey, false)
		u.bf.pC.lastRowid = pIn3.Integer()
		if u.bf.res == 0 {
			u.bf.pC.rowidIsValid = 1
		} else {
			u.bf.pC.rowidIsValid = 0
		}
		u.bf.pC.nullRow = 0
		u.bf.pC.cacheStatus = CACHE_STALE
		u.bf.pC.deferredMoveto = 0
		if u.bf.res != 0 {
			pc = pOp.p2 - 1
			assert( u.bf.pC.rowidIsValid == 0 )
		}
		u.bf.pC.seekResult = u.bf.res
	} else {
		//	This happens when an attempt to open a read cursor on the sqlite_master table returns SQLITE_EMPTY.
		pc = pOp.p2 - 1
		assert( u.bf.pC.rowidIsValid == 0 )
		u.bf.pC.seekResult = 0
	}

//	Opcode: Sequence P1 P2 * * *
//	Find the next available sequence number for cursor P1. Write the sequence number into register P2. The sequence number on the cursor is incremented after this instruction.  
case OP_Sequence:				//	out2-prerelease
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	assert( p.apCsr[pOp.p1] != 0 )
	pOut.Store(p.apCsr[pOp.p1].seqCount)
	p.apCsr[pOp.p1].seqCount++

/* Opcode: NewRowid P1 P2 P3 * *
**
** Get a new integer record number (a.k.a "rowid") used as the key to a table.
** The record number is not previously used as a key in the database
** table that cursor P1 points to.  The new record number is written
** written to register P2.
**
** If P3>0 then P3 is a register in the root frame of this VDBE that holds 
** the largest previously generated record number. No new record numbers are
** allowed to be less than this value. When this value reaches its maximum, 
** an SQLITE_FULL error is generated. The P3 register is updated with the '
** generated record number. This P3 mechanism is used to help implement the
** AUTOINCREMENT feature.
*/
case OP_NewRowid: {           /* out2-prerelease */
  u.bg.v = 0;
  u.bg.res = 0;
  assert( pOp.p1>=0 && pOp.p1<p.nCursor );
  u.bg.pC = p.apCsr[pOp.p1];
  assert( u.bg.pC!=0 );
  if( u.bg.pC.pCursor==0 ){
    /* The zero initialization above is all that is needed */
  }else{
    /* The next rowid or record number (different terms for the same
    ** thing) is obtained in a two-step algorithm.
    **
    ** First we attempt to find the largest existing rowid and add one
    ** to that.  But if the largest existing rowid is already the maximum
    ** positive integer, we have to fall through to the second
    ** probabilistic algorithm
    **
    ** The second algorithm is to select a rowid at random and see if
    ** it already exists in the table.  If it does not exist, we have
    ** succeeded.  If the random rowid does exist, we select a new one
    ** and try again, up to 100 times.
    */
    assert( u.bg.pC.isTable );

#ifdef SQLITE_32BIT_ROWID
#   define MAX_ROWID 0x7fffffff
#else
    /* Some compilers complain about constants of the form 0x7fffffffffffffff.
    ** Others complain about 0x7ffffffffffffffffLL.  The following macro seems
    ** to provide the constant while making all compilers happy.
    */
#   define MAX_ROWID  (int64)( (((uint64)0x7fffffff)<<32) | (uint64)0xffffffff )
#endif

    if( !u.bg.pC.useRandomRowid ){
      u.bg.v = sqlite3BtreeGetCachedRowid(u.bg.pC.pCursor);
      if( u.bg.v==0 ){
        rc = sqlite3BtreeLast(u.bg.pC.pCursor, &u.bg.res);
        if( rc!=SQLITE_OK ){
          goto abort_due_to_error;
        }
        if( u.bg.res ){
          u.bg.v = 1;   /* IMP: R-61914-48074 */
        }else{
          assert( sqlite3BtreeCursorIsValid(u.bg.pC.pCursor) );
          rc = sqlite3BtreeKeySize(u.bg.pC.pCursor, &u.bg.v);
          assert( rc==SQLITE_OK );   /* Cannot fail following BtreeLast() */
          if( u.bg.v>=MAX_ROWID ){
            u.bg.pC.useRandomRowid = 1;
          }else{
            u.bg.v++;   /* IMP: R-29538-34987 */
          }
        }
      }

#ifndef SQLITE_OMIT_AUTOINCREMENT
      if( pOp.p3 ){
        /* Assert that P3 is a valid memory cell. */
        assert( pOp.p3>0 );
        if( p.pFrame ){
          for(u.bg.pFrame=p.pFrame; u.bg.pFrame.pParent; u.bg.pFrame=u.bg.pFrame.pParent);
          /* Assert that P3 is a valid memory cell. */
          assert( pOp.p3<=u.bg.pFrame.nMem );
          u.bg.pMem = &u.bg.pFrame.aMem[pOp.p3];
        }else{
          /* Assert that P3 is a valid memory cell. */
          assert( pOp.p3<=p.nMem );
          u.bg.pMem = &aMem[pOp.p3];
          memAboutToChange(p, u.bg.pMem);
        }
        assert( memIsValid(u.bg.pMem) );

        REGISTER_TRACE(pOp.p3, u.bg.pMem);
        u.bg.pMem.Integerify()
        assert( (u.bg.pMem.flags & )!=0 );  /* mem(P3) holds an integer */
        if u.bg.pMem.Integer() == MAX_ROWID || u.bg.pC.useRandomRowid {
          rc = SQLITE_FULL;   /* IMP: R-12275-61338 */
          goto abort_due_to_error;
        }
        if x := u.bg.pMem.Integer() + 1; u.bg.v < x {
			u.bg.v = x
		}
        u.bg.pMem.Store(u.bg.v)
      }
#endif

      sqlite3BtreeSetCachedRowid(u.bg.pC.pCursor, u.bg.v<MAX_ROWID ? u.bg.v+1 : 0);
    }
    if( u.bg.pC.useRandomRowid ){
      /* IMPLEMENTATION-OF: R-07677-41881 If the largest ROWID is equal to the
      ** largest possible integer (9223372036854775807) then the database
      ** engine starts picking positive candidate ROWIDs at random until
      ** it finds one that is not previously used. */
      assert( pOp.p3==0 );  /* We cannot be in random rowid mode if this is
                             ** an AUTOINCREMENT table. */
      /* on the first attempt, simply do one more than previous */
      u.bg.v = lastRowid;
      u.bg.v &= (MAX_ROWID>>1); /* ensure doesn't go negative */
      u.bg.v++; /* ensure non-zero */
      u.bg.cnt = 0;
      for ((u.bg.res, rc = u.bg.pC.pCursor.BtreeMovetoUnpacked(0, uint64(u.bg.v), false)) == SQLITE_OK) && (u.bg.res == 0) && (++u.bg.cnt < 100) {
        //	collision - try another random rowid
        rand.Read(&u.bg.v)
        if u.bg.cnt < 5 {
          /* try "small" random rowids for the initial attempts */
          u.bg.v &= 0xffffff;
        }else{
          u.bg.v &= (MAX_ROWID>>1); /* ensure doesn't go negative */
        }
        u.bg.v++; /* ensure non-zero */
      }
      if( rc==SQLITE_OK && u.bg.res==0 ){
        rc = SQLITE_FULL;   /* IMP: R-38219-53002 */
        goto abort_due_to_error;
      }
      assert( u.bg.v>0 );  /* EV: R-40812-03570 */
    }
    u.bg.pC.rowidIsValid = 0;
    u.bg.pC.deferredMoveto = 0;
    u.bg.pC.cacheStatus = CACHE_STALE;
  }
  pOut.Store(u.bg.v)
  break;
}

//	Opcode: Insert P1 P2 P3 P4 P5
//	Write an entry into the table of cursor P1. A new entry is created if it doesn't already exist or the data for an existing entry is overwritten. The data is the value MEM_Blob stored in register number P2. The key is stored in register P3.
//	If the OPFLAG_NCHANGE flag of P5 is set, then the row change count is incremented (otherwise not). If the OPFLAG_LASTROWID flag of P5 is set, then rowid is stored for subsequent return by the sqlite3_last_insert_rowid() function (otherwise it is unmodified).
//	If the OPFLAG_USESEEKRESULT flag of P5 is set and if the result of the last seek operation (OP_NotExists) was a success, then this operation will not attempt to find the appropriate row before doing the insert but will instead overwrite the row that the cursor is currently pointing to. Presumably, the prior OP_NotExists opcode has already positioned the cursor correctly. This is an optimization that boosts performance by avoiding redundant seeks.
//	If the OPFLAG_ISUPDATE flag is set, then this opcode is part of an UPDATE operation. Otherwise (if the flag is clear) then this opcode is part of an INSERT operation. The difference is only important to the update hook.
//	Parameter P4 may point to a string containing the table-name, or may be NULL. If it is not NULL, then the update-hook (sqlite3.xUpdateCallback) is invoked following a successful insert.
//	(WARNO: If P1 is a pseudo-cursor and P2 is dynamically allocated, then ownership of P2 is transferred to the pseudo-cursor and register P2 becomes ephemeral. If the cursor is changed, the value of register P2 will then change. Make sure this does not cause any problems.)
//	This instruction only works on tables. The equivalent instruction for indices is OP_IdxInsert.
//
//	Opcode: InsertInt P1 P2 P3 P4 P5
//	This works exactly like OP_Insert except that the key is the integer value P3, not the value of the integer stored in register P3.
case OP_Insert:			fallthrough
case OP_InsertInt:
	u.bh.pData = &aMem[pOp.p2]
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	assert( memIsValid(u.bh.pData) )
	u.bh.pC = p.apCsr[pOp.p1]
	assert( u.bh.pC != nil )
	assert( u.bh.pC.pCursor != nil )
	assert( u.bh.pC.pseudoTableReg == 0 )
	assert( u.bh.pC.isTable )
	REGISTER_TRACE(pOp.p2, u.bh.pData)

	if pOp.opcode == OP_Insert {
		u.bh.pKey = &aMem[pOp.p3]
		assert( u.bh.pKey.flags.(int64) )
		assert( memIsValid(u.bh.pKey) )
		REGISTER_TRACE(pOp.p3, u.bh.pKey)
		u.bh.iKey = u.bh.pKey.Integer()
	} else {
		assert( pOp.opcode == OP_InsertInt )
		u.bh.iKey = pOp.p3
	}
	if pOp.p5 & OPFLAG_NCHANGE {
		p.nChange++
	}
	if pOp.p5 & OPFLAG_LASTROWID {
		db.lastRowid = lastRowid = u.bh.iKey
	}
	if u.bh.pData.Value == nil {
		u.bh.pData.z = ""
		u.bh.pData.n = 0
	} else {
		assert( u.bh.pData.flags & (MEM_Blob | MEM_Str) )
	}
	if pOp.p5 & OPFLAG_USESEEKRESULT != 0 {
		u.bh.seekResult = u.bh.pC.seekResult
	} else {
		u.bh.seekResult = 0
	}
	if v, ok := u.bh.pData.(Zeroes); ok {
		u.bh.nZero = u.bh.pData.u.nZero
	} else {
		u.bh.nZero = 0
	}
	sqlite3BtreeSetCachedRowid(u.bh.pC.pCursor, 0)
	rc = sqlite3BtreeInsert(u.bh.pC.pCursor, 0, u.bh.iKey, u.bh.pData.z, u.bh.pData.n, u.bh.nZero, pOp.p5 & OPFLAG_APPEND, u.bh.seekResult)
	u.bh.pC.rowidIsValid = 0
	u.bh.pC.deferredMoveto = 0
	ucacheStatus = CACHE_STALE

	//	Invoke the update-hook if required.
	if rc == SQLITE_OK && db.xUpdateCallback != nil && pOp.p4.z != "" {
		u.bh.zDb = db.Databases[u.bh.pC.iDb].Name
		u.bh.zTbl = pOp.p4.z
		if pOp.p5 & OPFLAG_ISUPDATE {
			u.bh.op = SQLITE_UPDATE
		} else {
			u.bh.op = SQLITE_INSERT
		}
		assert( u.bh.pC.isTable )
		db.xUpdateCallback(db.pUpdateArg, u.bh.op, u.bh.zDb, u.bh.zTbl, u.bh.iKey)
		assert( u.bh.pC.iDb >= 0 )
	}

//	Opcode: Delete P1 P2 * P4 *
//	Delete the record at which the P1 cursor is currently pointing.
//	The cursor will be left pointing at either the next or the previous record in the table. If it is left pointing at the next record, then the next Next instruction will be a no-op. Hence it is OK to delete a record from within an Next loop.
//	If the OPFLAG_NCHANGE flag of P2 is set, then the row change count is incremented (otherwise not).
//	P1 must not be pseudo-table. It has to be a real table with multiple rows.
//	If P4 is not NULL, then it is the name of the table that P1 is pointing to. The update hook will be invoked, if it exists. If P4 is not NULL then the P1 cursor must have been positioned using OP_NotFound prior to invoking this opcode.
case OP_Delete:
	u.bi.iKey = 0
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bi.pC = p.apCsr[pOp.p1]
	assert( u.bi.pC != nil )
	assert( u.bi.pC.pCursor != nil )			//	Only valid for real tables, no pseudotables

	//	If the update-hook will be invoked, set u.bi.iKey to the rowid of the row being deleted.
	if db.xUpdateCallback != nil && pOp.p4.z != "" {
		assert( u.bi.pC.isTable )
		assert( u.bi.pC.rowidIsValid )			//	lastRowid set by previous OP_NotFound
		u.bi.iKey = u.bi.pC.lastRowid
	}

	//	The OP_Delete opcode always follows an OP_NotExists or OP_Last or OP_Column on the same table without any intervening operations that might move or invalidate the cursor. Hence cursor u.bi.pC is always pointing to the row to be deleted and the sqlite3VdbeCursorMoveto() operation below is always a no-op and cannot fail. We will run it anyhow, though, to guard against future changes to the code generator.
	assert( u.bi.pC.deferredMoveto == 0 )
	if rc = sqlite3VdbeCursorMoveto(u.bi.pC); rc != SQLITE_OK {
		goto abort_due_to_error
	}

	sqlite3BtreeSetCachedRowid(u.bi.pC.pCursor, 0)
	rc = sqlite3BtreeDelete(u.bi.pC.pCursor)
	u.bi.pC.cacheStatus = CACHE_STALE

	//	Invoke the update-hook if required.
	if rc == SQLITE_OK && db.xUpdateCallback != nil && pOp.p4.z != "" {
		zDb := db.Databases[u.bi.pC.iDb].Name
		zTbl := pOp.p4.z
		db.xUpdateCallback(db.pUpdateArg, SQLITE_DELETE, zDb, zTbl, u.bi.iKey)
		assert( u.bi.pC.iDb >= 0 )
	}
	if pOp.p2 & OPFLAG_NCHANGE != 0 {
		p.nChange++
	}

//	Opcode: ResetCount * * * * *
//	The value of the change counter is copied to the database handle change counter (returned by subsequent calls to sqlite3_changes()). Then the VMs internal change counter resets to 0. This is used by trigger programs.
case OP_ResetCount:
	db.VdbeSetChanges(p.nChange)
	p.nChange = 0

//	Opcode: SorterCompare P1 P2 P3
//	P1 is a sorter cursor. This instruction compares the record blob in register P3 with the entry that the sorter cursor currently points to. If, excluding the rowid fields at the end, the two records are a match, fall through to the next instruction. Otherwise, jump to instruction P2.
case OP_SorterCompare:
	u.bj.pC = p.apCsr[pOp.p1]
	assert( isSorter(u.bj.pC) )
	pIn3 = &aMem[pOp.p3]
	if u.bj.res = u.bj.pC.SorterCompare(pIn3); u.bj.res {
		pc = pOp.p2 - 1
	}

//	Opcode: SorterData P1 P2 * * *
//	Write into register P2 the current sorter data for sorter cursor P1.
case OP_SorterData: {
	pOut = &aMem[pOp.p2]
	u.bk.pC = p.apCsr[pOp.p1]
	assert( u.bk.pC.isSorter )
	rc = u.bk.pC.SorterRowkey(pOut)

//	Opcode: RowData P1 P2 * * *
//	Write into register P2 the complete row data for cursor P1. There is no interpretation of the data. It is just copied onto the P2 register exactly as it is found in the database file.
//	If the P1 cursor must be pointing to a valid row (not a NULL row) of a real table, not a pseudo-table.
//
//	Opcode: RowKey P1 P2 * * *
//	Write into register P2 the complete row key for cursor P1. There is no interpretation of the data. The key is copied onto the P3 register exactly as it is found in the database file.
//	If the P1 cursor must be pointing to a valid row (not a NULL row) of a real table, not a pseudo-table.
case OP_RowKey:			fallthrough
case OP_RowData:
	pOut = &aMem[pOp.p2]
	memAboutToChange(p, pOut)

	//	Note that RowKey and RowData are really exactly the same instruction
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bl.pC = p.apCsr[pOp.p1]
	assert( u.bl.pC.isSorter == 0 )
	assert( u.bl.pC.isTable || pOp.opcode != OP_RowData )
	assert( u.bl.pC.isIndex || pOp.opcode == OP_RowData )
	assert( u.bl.pC != nil )
	assert( u.bl.pC.nullRow == 0 )
	assert( u.bl.pC.pseudoTableReg == 0 )
	assert( !u.bl.pC.isSorter )
	assert( u.bl.pC.pCursor != 0 )
	u.bl.pCrsr = u.bl.pC.pCursor
	assert( sqlite3BtreeCursorIsValid(u.bl.pCrsr) )

	//	The OP_RowKey and OP_RowData opcodes always follow OP_NotExists or OP_Rewind/Op_Next with no intervening instructions that might invalidate the cursor. Hence the following sqlite3VdbeCursorMoveto() call is always a no-op and can never fail. But we leave it in place as a safety.
	assert( u.bl.pC.deferredMoveto == 0 )
	if rc = sqlite3VdbeCursorMoveto(u.bl.pC); rc != SQLITE_OK {
		goto abort_due_to_error
	}

	if u.bl.pC.isIndex {
		assert( !u.bl.pC.isTable )
		VVA_ONLY(rc =) sqlite3BtreeKeySize(u.bl.pCrsr, &u.bl.n64)
		assert( rc == SQLITE_OK )				//	True because of CursorMoveto() call above
		if u.bl.n64 < 0 {
			goto too_big
		}
		u.bl.n = uint32(u.bl.n64)
	} else {
		VVA_ONLY(rc =) sqlite3BtreeDataSize(u.bl.pCrsr, &u.bl.n)
		assert( rc == SQLITE_OK )				//	DataSize() cannot fail
		if u.bl.n < 0 {
			goto too_big
		}
	}
	if sqlite3VdbeMemGrow(pOut, u.bl.n, 0) {
		goto no_mem
	}
	pOut.n = u.bl.n
	pOut.SetTypeFlag(MEM_Blob)
	if u.bl.pC.isIndex {
		rc = sqlite3BtreeKey(u.bl.pCrsr, 0, u.bl.n, pOut.z)
	} else {
		rc = sqlite3BtreeData(u.bl.pCrsr, 0, u.bl.n, pOut.z)
	}
	pOut.enc = SQLITE_UTF8						//	In case the blob is ever cast to text
	UPDATE_MAX_BLOBSIZE(pOut)

//	Opcode: Rowid P1 P2 * * *
//	Store in register P2 an integer which is the key of the table entry that P1 is currently point to.
//	P1 can be either an ordinary table or a virtual table. There used to be a separate OP_VRowid opcode for use with virtual tables, but this one opcode now works for both table types.
case OP_Rowid:					//	out2-prerelease
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bm.pC = p.apCsr[pOp.p1]
	assert( u.bm.pC != nil )
	assert( u.bm.pC.pseudoTableReg == 0 )
	if u.bm.pC.nullRow {
		pOut.Value = nil
	} else {
		switch {
		case u.bm.pC.deferredMoveto:
			u.bm.v = u.bm.pC.movetoTarget
		case u.bm.pC.pVtabCursor:
			u.bm.pVtab = u.bm.pC.pVtabCursor.pVtab
			u.bm.Callbacks = u.bm.pVtab.Callbacks
			assert( u.bm.Callbacks.xRowid )
			rc = u.bm.Callbacks.xRowid(u.bm.pC.pVtabCursor, &u.bm.v)
			importVtabErrMsg(p, u.bm.pVtab)
		default:
			assert( u.bm.pC.pCursor != nil )
			if rc = sqlite3VdbeCursorMoveto(u.bm.pC); rc != SQLITE_OK {
				goto abort_due_to_error
			}
			if u.bm.pC.rowidIsValid {
				u.bm.v = u.bm.pC.lastRowid
			} else {
				rc = sqlite3BtreeKeySize(u.bm.pC.pCursor, &u.bm.v)
				assert( rc == SQLITE_OK )			//	Always so because of CursorMoveto() above
			}
		}
		pOut.Store(u.bm.v)
	}

//	Opcode: NullRow P1 * * * *
//	Move the cursor P1 to a null row. Any OP_Column operations that occur while the cursor is on the null row will always write a NULL.
case OP_NullRow:
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bn.pC = p.apCsr[pOp.p1]
	assert( u.bn.pC != nil )
	u.bn.pC.nullRow = 1
	u.bn.pC.rowidIsValid = 0
	assert( u.bn.pC.pCursor || u.bn.pC.pVtabCursor )
	if u.bn.pC.pCursor != nil {
		sqlite3BtreeClearCursor(u.bn.pC.pCursor)
	}

//	Opcode: Last P1 P2 * * *
//	The next use of the Rowid or Column or Next instruction for P1 will refer to the last entry in the database table or index. If the table or index is empty and P2>0, then jump immediately to P2. If P2 is 0 or if the table or index is not empty, fall through to the following instruction.
case OP_Last:					//	jump
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bo.pC = p.apCsr[pOp.p1]
	assert( u.bo.pC != nil )
	u.bo.pCrsr = u.bo.pC.pCursor
	u.bo.res = 0
	if u.bo.pCrsr != nil {
		rc = sqlite3BtreeLast(u.bo.pCrsr, &u.bo.res)
	}
	u.bo.pC.nullRow = byte(u.bo.res)
	u.bo.pC.deferredMoveto = 0
	u.bo.pC.rowidIsValid = 0
	u.bo.pC.cacheStatus = CACHE_STALE
	if pOp.p2 > 0 && u.bo.res {
		pc = pOp.p2 - 1
	}

//	Opcode: Sort P1 P2 * * *
//	This opcode does exactly the same thing as OP_Rewind except that it increments an undocumented global variable used for testing.
//	Sorting is accomplished by writing records into a sorting index, then rewinding that index and playing it back from beginning to end. We use the OP_Sort opcode instead of OP_Rewind to do the rewinding so that the global variable will be incremented and regression tests can determine whether or not the optimizer is correctly optimizing out sorts.
case OP_SorterSort:		fallthrough			//	jump
case OP_Sort:								//	jump
	p.aCounter[SQLITE_STMTSTATUS_SORT-1]++
	fallthrough

//	Opcode: Rewind P1 P2 * * *
//	The next use of the Rowid or Column or Next instruction for P1 will refer to the first entry in the database table or index.
//	If the table or index is empty and P2>0, then jump immediately to P2.
//	If P2 is 0 or if the table or index is not empty, fall through to the following instruction.
case OP_Rewind: {			//	jump
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bp.pC = p.apCsr[pOp.p1]
	assert( u.bp.pC != nil )
	assert( u.bp.pC.isSorter == (pOp.opcode == OP_SorterSort) )
	u.bp.res = true
	if isSorter(u.bp.pC) {
		rc = sqlite3VdbeSorterRewind(db, u.bp.pC, &u.bp.res)
	} else {
		u.bp.pCrsr = u.bp.pC.pCursor
		assert( u.bp.pCrsr )
		rc = sqlite3BtreeFirst(u.bp.pCrsr, &u.bp.res)
		if u.bp.res == 0 {
			u.bp.pC.atFirst = true
		} else {
			u.bp.pC.atFirst = false
		}
		u.bp.pC.deferredMoveto = 0
		u.bp.pC.cacheStatus = CACHE_STALE
		u.bp.pC.rowidIsValid = 0
	}
	u.bp.pC.nullRow = byte(u.bp.res)
	assert( pOp.p2 > 0 && pOp.p2 < len(p.Program) )
	if u.bp.res {
		pc = pOp.p2 - 1
	}
}

//	Opcode: Next P1 P2 * P4 P5
//	Advance cursor P1 so that it points to the next key/data pair in its table or index. If there are no more key/value pairs then fall through to the following instruction. But if the cursor advance was successful, jump immediately to P2.
//	The P1 cursor must be for a real table, not a pseudo-table.
//	P4 is always of type P4_ADVANCE. The function pointer points to sqlite3BtreeNext().
//	If P5 is positive and the jump is taken, then event counter number P5-1 in the prepared statement is incremented.
//
//	Opcode: Prev P1 P2 * * P5
//	Back up cursor P1 so that it points to the previous key/data pair in its table or index. If there is no previous key/value pairs then fall through to the following instruction. But if the cursor backup was successful, jump immediately to P2.
//	The P1 cursor must be for a real table, not a pseudo-table.
//	P4 is always of type P4_ADVANCE. The function pointer points to sqlite3BtreePrevious().
//	If P5 is positive and the jump is taken, then event counter number P5-1 in the prepared statement is incremented.
case OP_SorterNext:		fallthrough		//	jump
case OP_Prev:			fallthrough		//	jump
case OP_Next:							//	jump
	CHECK_FOR_INTERRUPT
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	assert( pOp.p5 <= ArraySize(p.aCounter) )
	if u.bq.pC = p.apCsr[pOp.p1]; u.bq.pC == 0 {
		break			//	See ticket #2273
	}
	assert( u.bq.pC.isSorter == (pOp.opcode == OP_SorterNext) )
	if isSorter(u.bq.pC) {
		assert( pOp.opcode == OP_SorterNext )
		u.bq.res, rc = u.bq.pC.SorterNext()
	} else {
		u.bq.res = 1
		assert( u.bq.pC.deferredMoveto == 0 )
		assert( u.bq.pC.pCursor )
		assert( pOp.opcode != OP_Next || pOp.p4.xAdvance == sqlite3BtreeNext )
		assert( pOp.opcode != OP_Prev || pOp.p4.xAdvance == sqlite3BtreePrevious )
		rc = pOp.p4.xAdvance(u.bq.pC.pCursor, &u.bq.res)
	}
	u.bq.pC.nullRow = byte(u.bq.res)
	u.bq.pC.cacheStatus = CACHE_STALE
	if u.bq.res == 0 {
		pc = pOp.p2 - 1
		if pOp.p5 {
			p.aCounter[pOp.p5 - 1]++
		}
	}
	u.bq.pC.rowidIsValid = 0

/* Opcode: IdxInsert P1 P2 P3 * P5
**
** Register P2 holds an SQL index key made using the
** MakeRecord instructions.  This opcode writes that key
** into the index P1.  Data for the entry is nil.
**
** P3 is a flag that provides a hint to the b-tree layer that this
** insert is likely to be an append.
**
** This instruction only works for indices.  The equivalent instruction
** for tables is OP_Insert.
*/
case OP_SorterInsert:       /* in2 */

case OP_IdxInsert: {        /* in2 */
  assert( pOp.p1>=0 && pOp.p1<p.nCursor );
  u.br.pC = p.apCsr[pOp.p1];
  assert( u.br.pC!=0 );
  assert( u.br.pC.isSorter==(pOp.opcode==OP_SorterInsert) );
  pIn2 = &aMem[pOp.p2];
  assert( pIn2.flags & MEM_Blob );
  u.br.pCrsr = u.br.pC.pCursor;
  if( u.br.pCrsr!=0 ){
    assert( u.br.pC.isTable==0 );
    rc = ExpandBlob(pIn2);
    if( rc==SQLITE_OK ){
      if( isSorter(u.br.pC) ){
        rc = sqlite3VdbeSorterWrite(db, u.br.pC, pIn2);
      }else{
        u.br.nKey = pIn2.n;
        u.br.zKey = pIn2.z;
        rc = sqlite3BtreeInsert(u.br.pCrsr, u.br.zKey, u.br.nKey, "", 0, 0, pOp.p3,
            ((pOp.p5 & OPFLAG_USESEEKRESULT) ? u.br.pC.seekResult : 0)
            );
        assert( u.br.pC.deferredMoveto==0 );
        u.br.pC.cacheStatus = CACHE_STALE;
      }
    }
  }
  break;
}

//	Opcode: IdxDelete P1 P2 P3 * *
//	The content of P3 registers starting at register P2 form an unpacked index key. This opcode removes that entry from the index opened by cursor P1.
case OP_IdxDelete:
	assert( pOp.p3 > 0 )
	assert( pOp.p2 > 0 && pOp.p2 + pOp.p3 <= p.nMem + 1 )
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bs.pC = p.apCsr[pOp.p1]
	assert( u.bs.pC != nil )
	if u.bs.pCrsr = u.bs.pC.pCursor; u.bs.pCrsr != nil {
		u.bs.r.pKeyInfo = u.bs.pC.pKeyInfo
		u.bs.r.nField = uint16(pOp.p3)
		u.bs.r.flags = 0
		u.bs.r.aMem = &aMem[pOp.p2]
		if u.bs.res, rc = u.bs.pCrsr.BtreeMovetoUnpacked(&u.bs.r, 0, false); rc == SQLITE_OK && u.bs.res == 0 {
			rc = sqlite3BtreeDelete(u.bs.pCrsr)
		}
		assert( u.bs.pC.deferredMoveto == 0 )
		u.bs.pC.cacheStatus = CACHE_STALE
	}

//	Opcode: IdxRowid P1 P2 * * *
//	Write into register P2 an integer which is the last entry in the record at the end of the index key pointed to by cursor P1. This integer should be the rowid of the table entry to which this index entry points.
//	See also: Rowid, MakeRecord.
case OP_IdxRowid:				//	out2-prerelease
	assert( pOp.p1 >= 0 && pOp.p1 < p.nCursor )
	u.bt.pC = p.apCsr[pOp.p1]
	assert( u.bt.pC != nil )
	u.bt.pCrsr = u.bt.pC.pCursor
	pOut.Value = nil
	if u.bt.pCrsr != nil {
		if rc = sqlite3VdbeCursorMoveto(u.bt.pC) {
			goto abort_due_to_error
		}
		assert( u.bt.pC.deferredMoveto == 0 )
		assert( u.bt.pC.isTable == 0 )
		if !u.bt.pC.nullRow {
			if rc = sqlite3VdbeIdxRowid(db, u.bt.pCrsr, &u.bt.rowid); rc != SQLITE_OK {
				goto abort_due_to_error
			}
			pOut.Store(u.bt.rowid)
		}
	}

/* Opcode: IdxGE P1 P2 P3 P4 P5
**
** The P4 register values beginning with P3 form an unpacked index 
** key that omits the ROWID.  Compare this key value against the index 
** that P1 is currently pointing to, ignoring the ROWID on the P1 index.
**
** If the P1 index entry is greater than or equal to the key value
** then jump to P2.  Otherwise fall through to the next instruction.
**
** If P5 is non-zero then the key value is increased by an epsilon 
** prior to the comparison.  This make the opcode work like IdxGT except
** that if the key from register P3 is a prefix of the key in the cursor,
** the result is false whereas it would be true with IdxGT.
*/
/* Opcode: IdxLT P1 P2 P3 P4 P5
**
** The P4 register values beginning with P3 form an unpacked index 
** key that omits the ROWID.  Compare this key value against the index 
** that P1 is currently pointing to, ignoring the ROWID on the P1 index.
**
** If the P1 index entry is less than the key value then jump to P2.
** Otherwise fall through to the next instruction.
**
** If P5 is non-zero then the key value is increased by an epsilon prior 
** to the comparison.  This makes the opcode work like IdxLE.
*/
case OP_IdxLT:          /* jump */
case OP_IdxGE: {        /* jump */
  assert( pOp.p1>=0 && pOp.p1<p.nCursor );
  u.bu.pC = p.apCsr[pOp.p1];
  assert( u.bu.pC!=0 );
  assert( u.bu.pC.isOrdered );
  if( u.bu.pC.pCursor!=0 ){
    assert( u.bu.pC.deferredMoveto==0 );
    assert( pOp.p5==0 || pOp.p5==1 );
    assert( pOp.p4type==P4_INT32 );
    u.bu.r.pKeyInfo = u.bu.pC.pKeyInfo;
    u.bu.r.nField = (uint16)pOp.p4.i;
    if( pOp.p5 ){
      u.bu.r.flags = UNPACKED_INCRKEY | UNPACKED_PREFIX_MATCH;
    }else{
      u.bu.r.flags = UNPACKED_PREFIX_MATCH;
    }
    u.bu.r.aMem = &aMem[pOp.p3];
    rc = sqlite3VdbeIdxKeyCompare(u.bu.pC, &u.bu.r, &u.bu.res);
    if( pOp.opcode==OP_IdxLT ){
      u.bu.res = -u.bu.res;
    }else{
      assert( pOp.opcode==OP_IdxGE );
      u.bu.res++;
    }
    if( u.bu.res>0 ){
      pc = pOp.p2 - 1 ;
    }
  }
  break;
}

//	Opcode: Destroy P1 P2 P3 * *
//	Delete an entire database table or index whose root page in the database file is given by P1.
//	The table being destroyed is in the main database file if P3 == 0. If P3 == 1 then the table to be clear is in the auxiliary database file that is used to store tables create using CREATE TEMPORARY TABLE.
//	If AUTOVACUUM is enabled then it is possible that another root page might be moved into the newly deleted root page in order to keep all root pages contiguous at the beginning of the database. The former value of the root page that moved - its value before the move occurred - is stored in register P2. If no page movement was required (because the table being dropped was already the last one in the database) then a zero is stored in register P2. If AUTOVACUUM is disabled then a zero is stored in register P2.
//	See also: Clear
case OP_Destroy:					//	out2-prerelease
	u.bv.iCnt = 0
	for u.bv.pVdbe = db.pVdbe; u.bv.pVdbe != nil; u.bv.pVdbe = u.bv.pVdbe.Next {
		if u.bv.pVdbe.magic == VDBE_MAGIC_RUN && u.bv.pVdbe.inVtabMethod < 2 && u.bv.pVdbe.pc >= 0 {
			u.bv.iCnt++
		}
	}
	pOut.Value = nil
	if u.bv.iCnt > 1 {
		rc = SQLITE_LOCKED
		p.errorAction = OE_Abort
	} else {
		u.bv.iDb = pOp.p3
		assert( u.bv.iCnt == 1 )
		assert( p.btreeMask & (yDbMask(1) << u.bv.iDb) != 0 )
		rc = sqlite3BtreeDropTable(db.Databases[u.bv.iDb].pBt, pOp.p1, &u.bv.iMoved)
		pOut.Store(u.bv.iMoved)
		if rc == SQLITE_OK && u.bv.iMoved != 0 {
			db.RootPageMoved(u.bv.iDb, u.bv.iMoved, pOp.p1)
			//	All OP_Destroy operations occur on the same btree
			assert( resetSchemaOnFault == 0 || resetSchemaOnFault == u.bv.iDb + 1 )
			resetSchemaOnFault = u.bv.iDb + 1
		}
	}

//	Opcode: Clear P1 P2 P3
//	Delete all contents of the database table or index whose root page in the database file is given by P1. But, unlike Destroy, do not remove the table or index from the database file.
//	The table being clear is in the main database file if P2 == 0. If P2 == 1 then the table to be clear is in the auxiliary database file that is used to store tables create using CREATE TEMPORARY TABLE.
//	If the P3 value is non-zero, then the table referred to must be an intkey table (an SQL table, not an index). In this case the row change count is incremented by the number of rows in the table being cleared. If P3 is greater than zero, then the value stored in register P3 is also incremented by the number of rows in the table being cleared.
//	See also: Destroy
case OP_Clear:
	u.bw.nChange = 0
	assert( p.btreeMask & (yDbMask(1) << pOp.p2) != 0 )
	if pOp.p3 != 0 {
		rc = sqlite3BtreeClearTable(db.Databases[pOp.p2].pBt, pOp.p1, &u.bw.nChange)
	} else {
		rc = sqlite3BtreeClearTable(db.Databases[pOp.p2].pBt, pOp.p1, 0)
	}
	if pOp.p3 {
		p.nChange += u.bw.nChange
		if pOp.p3 > 0 {
			assert( memIsValid(&aMem[pOp.p3]) )
			memAboutToChange(p, &aMem[pOp.p3])
			aMem[pOp.p3].AddInteger(u.bw.nChange)
		}
	}

//	Opcode: CreateTable P1 P2 * * *
//	Allocate a new table in the main database file if P1 == 0 or in the auxiliary database file if P1 == 1 or in an attached database if P1 > 1. Write the root page number of the new table into register P2
//	The difference between a table and an index is this: A table must have a 4-byte integer key and can have arbitrary data. An index has an arbitrary key but no data.
//
//	Opcode: CreateIndex P1 P2 * * *
//	Allocate a new index in the main database file if P1 == 0 or in the auxiliary database file if P1 == 1 or in an attached database if P1 > 1. Write the root page number of the new table into register P2.
//	See documentation on OP_CreateTable for additional information.
case OP_CreateIndex:	fallthrough		//	out2-prerelease
case OP_CreateTable:					//	out2-prerelease
	u.bx.pgno = 0
	assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
	assert( p.btreeMask & (yDbMask(1) << pOp.p1) != 0 )
	u.bx.pDb = &db.Databases[pOp.p1]
	assert( u.bx.pDb.pBt != 0 )
	if pOp.opcode == OP_CreateTable {
		u.bx.flags = BTREE_INTKEY
	} else {
		u.bx.flags = BTREE_BLOBKEY
	}
	rc = sqlite3BtreeCreateTable(u.bx.pDb.pBt, &u.bx.pgno, u.bx.flags)
	pOut.Store(u.bx.pgno)

/* Opcode: ParseSchema P1 * * P4 *
**
** Read and parse all entries from the SQLITE_MASTER table of database P1
** that match the WHERE clause P4. 
**
** This opcode invokes the parser to create a new virtual machine,
** then runs the new virtual machine.  It is thus a re-entrant opcode.
*/
case OP_ParseSchema: {
  /* Any prepared statement that invokes this opcode will hold mutexes
  ** on every btree.  This is a prerequisite for invoking
  ** sqlite3InitCallback().
  */
  u.by.iDb = pOp.p1;
  assert( u.by.iDb >= 0 && u.by.iDb < len(db.Databases) )
  assert( db.HasProperty(u.by.iDb, DB_SchemaLoaded) )
  /* Used to be a conditional */ {
    u.by.zMaster = SCHEMA_TABLE(u.by.iDb);
    u.by.initData.db = db;
    u.by.initData.iDb = pOp.p1;
    u.by.initData.pzErrMsg = &p.zErrMsg;
    u.by.zSql = fmt.Sprintf("SELECT name, rootpage, sql FROM '%v'.%v WHERE %v ORDER BY rowid", db.Databases[u.by.iDb].Name, u.by.zMaster, pOp.p4.z);
    if( u.by.zSql==0 ){
      rc = SQLITE_NOMEM;
    }else{
      assert( db.init.busy==0 );
      db.init.busy = 1;
      u.by.initData.rc = SQLITE_OK;
      assert( !db.mallocFailed );
      rc = sqlite3_exec(db, u.by.zSql, sqlite3InitCallback, &u.by.initData, 0);
      if( rc==SQLITE_OK ) rc = u.by.initData.rc;
      u.by.zSql = nil
      db.init.busy = 0;
    }
  }
  if rc != SQLITE_OK {
	  db.ResetInternalSchema(-1)
	}
  if( rc==SQLITE_NOMEM ){
    goto no_mem;
  }
  break;
}

/* Opcode: LoadAnalysis P1 * * * *
**
** Read the sqlite_stat1 table for database P1 and load the content
** of that table into the internal index hash table.  This will cause
** the analysis to be used when preparing all subsequent queries.
*/
case OP_LoadAnalysis: {
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
  rc = sqlite3AnalysisLoad(db, pOp.p1);
  break;  
}

//	Opcode: DropTable P1 * * P4 *
//	Remove the internal (in-memory) data structures that describe the table named P4 in database P1. This is called after a table is dropped in order to keep the internal representation of the schema consistent with what is on disk.
case OP_DropTable:
  db.UnlinkAndDeleteTable(pOp.p1, pOp.p4.z)

//	Opcode: DropIndex P1 * * P4 *
//	Remove the internal (in-memory) data structures that describe the index named P4 in database P1. This is called after an index is dropped in order to keep the internal representation of the schema consistent with what is on disk.
case OP_DropIndex:
  db.UnlinkAndDeleteIndex(pOp.p1, pOp.p4.z)

//	Opcode: DropTrigger P1 * * P4 *
//	Remove the internal (in-memory) data structures that describe the trigger named P4 in database P1. This is called after a trigger is dropped in order to keep the internal representation of the schema consistent with what is on disk.
case OP_DropTrigger:
	db.UnlinkAndDeleteTrigger(pOp.p1, pOp.p4.z)


/* Opcode: IntegrityCheck P1 P2 P3 * P5
**
** Do an analysis of the currently open database.  Store in
** register P1 the text of an error message describing any problems.
** If no problems are found, store a NULL in register P1.
**
** The register P3 contains the maximum number of allowed errors.
** At most reg(P3) errors will be reported.
** In other words, the analysis stops as soon as reg(P1) errors are 
** seen.  Reg(P1) is updated with the number of errors remaining.
**
** The root page numbers of all tables in the database are integer
** stored in reg(P1), reg(P1+1), reg(P1+2), ....  There are P2 tables
** total.
**
** If P5 is not zero, the check is done on the auxiliary database
** file, not the main database file.
**
** This opcode is used to implement the integrity_check pragma.
*/
case OP_IntegrityCheck: {
  u.bz.nRoot = pOp.p2;
  assert( u.bz.nRoot>0 );
  u.bz.aRoot = sqlite3DbMallocRaw(db, sizeof(int)*(u.bz.nRoot+1) );
  if( u.bz.aRoot==0 ) goto no_mem;
  assert( pOp.p3>0 && pOp.p3<=p.nMem );
  u.bz.pnErr = &aMem[pOp.p3];
  assert( u.bz.pnErr.flags.(int64) )
  assert( u.bz.pnErr.flags & (MEM_Str | MEM_Blob) == 0 )
  pIn1 = &aMem[pOp.p1];
  for(u.bz.j=0; u.bz.j<u.bz.nRoot; u.bz.j++){
    u.bz.aRoot[u.bz.j] = int(pIn1[u.bz.j].IntValue())
  }
  u.bz.aRoot[u.bz.j] = 0;
  assert( pOp.p5 < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<pOp.p5))!=0 );
  u.bz.z = sqlite3BtreeIntegrityCheck(db.Databases[pOp.p5].pBt, u.bz.aRoot, u.bz.nRoot, int(u.bz.pnErr.Integer()), &u.bz.nErr)
  u.bz.aRoot = nil
  u.bz.pnErr.AddInteger(-u.bz.nErr)
  pIn1.SetNull()
  if( u.bz.nErr==0 ){
    assert( u.bz.z==0 );
  }else if( u.bz.z==0 ){
    goto no_mem;
  }else{
    pIn1.SetStr(u.bz.z, SQLITE_UTF8, sqlite3_free);
  }
  UPDATE_MAX_BLOBSIZE(pIn1);
  sqlite3VdbeChangeEncoding(pIn1, encoding);
  break;
}

//	Opcode: RowSetAdd P1 P2 * * *
//	Insert the integer value held by register P2 into a boolean index held in register P1.
//	An assertion fails if P2 is not an integer.
case OP_RowSetAdd:					//	in1, in2
	pIn1 = &aMem[pOp.p1]
	pIn2 = &aMem[pOp.p2]
	assert( pIn2.flags.(int64) )
	if pIn1.flags & MEM_RowSet == 0 {
		sqlite3VdbeMemSetRowSet(pIn1)
		if pIn1.flags & MEM_RowSet == 0 {
			goto no_mem
		}
	}
	sqlite3RowSetInsert(pIn1.u.pRowSet, pIn2.Integer())

//	Opcode: RowSetRead P1 P2 P3 * *
//	Extract the smallest value from boolean index P1 and put that value into register P3. Or, if boolean index P1 is initially empty, leave P3 unchanged and jump to instruction P2.
case OP_RowSetRead: {       /* jump, in1, out3 */
	CHECK_FOR_INTERRUPT
	pIn1 = &aMem[pOp.p1]
	if pIn1.flags & MEM_RowSet == 0 || sqlite3RowSetNext(pIn1.u.pRowSet, &u.ca.val) == 0 {
		//	The boolean index is empty
		pIn1.SetNull()
		pc = pOp.p2 - 1
	} else {
		//	A value was pulled from the index
		aMem[pOp.p3].SetInt64(u.ca.val)
	}

//	Opcode: RowSetTest P1 P2 P3 P4
//	Register P3 is assumed to hold a 64-bit integer value. If register P1 contains a RowSet object and that RowSet object contains the value held in P3, jump to register P2. Otherwise, insert the integer in P3 into the RowSet and continue on to the next opcode.
//	The RowSet object is optimized for the case where successive sets of integers, where each set contains no duplicates. Each set of values is identified by a unique P4 value. The first set must have P4 == 0, the final set P4 =  -1. P4 must be either -1 or non-negative. For non-negative values of P4 only the lower 4 bits are significant.
//	This allows optimizations: (a) when P4 == 0 there is no need to test the rowset object for P3, as it is guaranteed not to contain it, (b) when P4 == -1 there is no need to insert the value, as it will never be tested for, and (c) when a value that is part of set X is inserted, there is no need to search to see if the same value was previously inserted as part of set X (only if it was previously inserted as part of some other set).
case OP_RowSetTest:						//	jump, in1, in3
	pIn1 = &aMem[pOp.p1]
	pIn3 = &aMem[pOp.p3]
	u.cb.iSet = pOp.p4.i
	assert( pIn3.flags.(int64) )

	//	If there is anything other than a rowset object in memory cell P1, delete it now and initialize P1 with an empty rowset
	if pIn1.flags & MEM_RowSet == 0 {
		sqlite3VdbeMemSetRowSet(pIn1)
		if pIn1.flags & MEM_RowSet == 0 {
			goto no_mem
		}
	}

	assert( pOp.p4type == P4_INT32 )
	assert( u.cb.iSet == -1 || u.cb.iSet >= 0 )
	if u.cb.iSet  != 0 {
		if u.cb.iSet >= 0 {
			u.cb.exists = sqlite3RowSetTest(pIn1.u.pRowSet, byte(u.cb.iSet & 0xf), pIn3.Value)
		} else {
			u.cb.exists = sqlite3RowSetTest(pIn1.u.pRowSet, 0xff, pIn3.Value)
		}
		if u.cb.exists != 0 {
			pc = pOp.p2 - 1
			break
		}
	}
	if u.cb.iSet >= 0 {
		sqlite3RowSetInsert(pIn1.u.pRowSet, pIn3.Value)
	}

//	Opcode: Program P1 P2 P3 P4 *
//	Execute the trigger program passed as P4 (type P4_SUBPROGRAM). 
//	P1 contains the address of the memory cell that contains the first memory cell in an array of values used as arguments to the sub-program. P2 contains the address to jump to if the sub-program throws an IGNORE exception using the RAISE() function. Register P3 contains the address of a memory cell in this (the parent) VM that is used to allocate the memory required by the sub-vdbe at runtime.
//	P4 is a pointer to the VM containing the trigger program.
case OP_Program: {		//	jump
	u.cc.SubProgram = pOp.p4.SubProgram
	u.cc.pRt = &aMem[pOp.p3]
	assert( len(u.cc.SubProgram) > 0 )

	//	If the p5 flag is clear, then recursive invocation of triggers is disabled for backwards compatibility (p5 is set if this sub-program is really a trigger, not a foreign key action, and the flag set and cleared by the "PRAGMA recursive_triggers" command is clear).
	//	It is recursive invocation of triggers, at the SQL level, that is disabled. In some cases a single trigger may generate more than one SubProgram (if the trigger may be executed with more than one different ON CONFLICT algorithm). SubProgram structures associated with a single trigger all have the same value for the SubProgram.token variable.
	if pOp.p5 != nil {
		u.cc.t = u.cc.SubProgram.token
		for u.cc.pFrame = p.pFrame; u.cc.pFrame != nil && u.cc.pFrame.token != u.cc.t; u.cc.pFrame = u.cc.pFrame.pParent {}
		if u.cc.pFrame {
			break
		}
	}

	if p.nFrame < 0 {
		rc = SQLITE_ERROR
		p.zErrMsg = "too many levels of trigger recursion"
		break
	}

	//	Register u.cc.pRt is used to store the memory required to save the state of the current program, and the memory required at runtime to execute the trigger program. If this trigger has been fired before, then u.cc.pRt is already allocated. Otherwise, it must be initialized.
	if u.cc.pRt.flags & MEM_Frame == 0 {
		//	SubProgram.nMem is set to the number of memory cells used by the program stored in SubProgram.Opcodes. As well as these, one memory cell is required for each cursor used by the program. Set local variable u.cc.nMem (and later, VdbeFrame.nChildMem) to this value.
		u.cc.nMem = u.cc.SubProgram.nMem + u.cc.SubProgram.nCsr
		u.cc.nByte = ROUND(sizeof(VdbeFrame), 8)
						+ u.cc.nMem * sizeof(Mem)
						+ u.cc.SubProgram.nCsr * sizeof(VdbeCursor *)
						+ u.cc.SubProgram.nOnce * sizeof(byte)
		u.cc.pFrame = sqlite3DbMallocZero(db, u.cc.nByte)
		if u.cc.pFrame == nil {
			goto no_mem
		}
		u.cc.pRt.Release()
		u.cc.pRt.flags = MEM_Frame
		u.cc.pRt.u.pFrame = u.cc.pFrame

		u.cc.pFrame.v = p
		u.cc.pFrame.nChildMem = u.cc.nMem
		u.cc.pFrame.nChildCsr = u.cc.SubProgram.nCsr
		u.cc.pFrame.pc = pc
		u.cc.pFrame.aMem = p.aMem
		u.cc.pFrame.nMem = p.nMem
		u.cc.pFrame.apCsr = p.apCsr
		u.cc.pFrame.nCursor = p.nCursor
		u.cc.pFrame.Program = p.Program

		u.cc.pFrame.token = u.cc.SubProgram.token
		u.cc.pFrame.aOnceFlag = p.aOnceFlag
		u.cc.pFrame.nOnceFlag = p.nOnceFlag

		u.cc.pEnd = &VdbeFrameMem(u.cc.pFrame)[u.cc.pFrame.nChildMem]
		for u.cc.pMem = VdbeFrameMem(u.cc.pFrame); u.cc.pMem != u.cc.pEnd; u.cc.pMem++ {
			u.cc.pMem.flags = MEM_Invalid
			u.cc.pMem.db = db
		}
	} else {
		u.cc.pFrame = u.cc.pRt.u.pFrame
		assert( u.cc.SubProgram.nMem + u.cc.SubProgram.nCsr == u.cc.pFrame.nChildMem )
		assert( u.cc.SubProgram.nCsr == u.cc.pFrame.nChildCsr )
		assert( pc == u.cc.pFrame.pc )
	}

	p.nFrame++
	u.cc.pFrame.pParent = p.pFrame
	u.cc.pFrame.lastRowid = lastRowid
	u.cc.pFrame.nChange = p.nChange
	p.nChange = 0
	p.pFrame = u.cc.pFrame
	p.aMem = aMem = &VdbeFrameMem(u.cc.pFrame)[-1]
	p.nMem = u.cc.pFrame.nChildMem
	p.nCursor = uint16(u.cc.pFrame.nChildCsr)
	p.apCsr = (VdbeCursor **)(&aMem[p.nMem + 1])
	Program = u.cc.Subprogram.Program
	p.Program = Program
	p.aOnceFlag = (byte *)(&p.apCsr[p.nCursor])
	p.nOnceFlag = u.cc.SubProgram.nOnce
	pc = -1
	memset(p.aOnceFlag, 0, p.nOnceFlag)
}

//	Opcode: Param P1 P2 * * *
//	This opcode is only ever present in sub-programs called via the OP_Program instruction. Copy a value currently stored in a memory cell of the calling (parent) frame to cell P2 in the current frames address space. This is used by trigger programs to access the new.* and old.* values.
//	The address of the cell in the parent frame is determined by adding the value of the P1 argument to the value of the P1 argument to the calling OP_Program instruction.
case OP_Param: {				//	out2-prerelease
	u.cd.pFrame = p.pFrame
	u.cd.pIn = &u.cd.pFrame.aMem[pOp.p1 + u.cd.pFrame.Program[u.cd.pFrame.pc].p1]
	sqlite3VdbeMemShallowCopy(pOut, u.cd.pIn, MEM_Ephem)
}

/* Opcode: FkCounter P1 P2 * * *
**
** Increment a "constraint counter" by P2 (P2 may be negative or positive).
** If P1 is non-zero, the database constraint counter is incremented 
** (deferred foreign key constraints). Otherwise, if P1 is zero, the 
** statement counter is incremented (immediate foreign key constraints).
*/
case OP_FkCounter: {
  if( pOp.p1 ){
    db.nDeferredCons += pOp.p2;
  }else{
    p.nFkConstraint += pOp.p2;
  }
  break;
}

/* Opcode: FkIfZero P1 P2 * * *
**
** This opcode tests if a foreign key constraint-counter is currently zero.
** If so, jump to instruction P2. Otherwise, fall through to the next 
** instruction.
**
** If P1 is non-zero, then the jump is taken if the database constraint-counter
** is zero (the one that counts deferred constraint violations). If P1 is
** zero, the jump is taken if the statement constraint-counter is zero
** (immediate foreign key constraint violations).
*/
case OP_FkIfZero: {         /* jump */
  if( pOp.p1 ){
    if( db.nDeferredCons==0 ) pc = pOp.p2-1;
  }else{
    if( p.nFkConstraint==0 ) pc = pOp.p2-1;
  }
  break;
}

#ifndef SQLITE_OMIT_AUTOINCREMENT
//	Opcode: MemMax P1 P2 * * *
//	P1 is a register in the root frame of this VM (the root frame is different from the current frame if this instruction is being executed within a sub-program). Set the value of register P1 to the maximum of its current value and the value in register P2.
//	This instruction throws an error if the memory cell is not initially an integer.
case OP_MemMax:					//	in2
	if p.pFrame != nil {
		for(u.ce.pFrame=p.pFrame; u.ce.pFrame.pParent; u.ce.pFrame=u.ce.pFrame.pParent);
		u.ce.pIn1 = &u.ce.pFrame.aMem[pOp.p1];
	} else {
		u.ce.pIn1 = &aMem[pOp.p1];
	}
	assert( memIsValid(u.ce.pIn1) )
	u.ce.pIn1.Integerify()
	pIn2 = &aMem[pOp.p2]
	pIn2.Integerify()
	if u.ce.pIn1.Value < pIn2.Value {
		u.ce.pIn1.Store(pIn2.Value)
	}
#endif /* SQLITE_OMIT_AUTOINCREMENT */

//	Opcode: IfPos P1 P2 * * *
//	If the value of register P1 is 1 or greater, jump to P2.
//	It is illegal to use this instruction on a register that does not contain an integer. An assertion fault will result if you try.
case OP_IfPos:					//	jump, in1
	pIn1 = &aMem[pOp.p1]
	assert( pIn1.flags.(int64) )
	if pIn1.Integer() > 0 {
		pc = pOp.p2 - 1
	}

//	Opcode: IfNeg P1 P2 * * *
//	If the value of register P1 is less than zero, jump to P2. 
//	It is illegal to use this instruction on a register that does not contain an integer. An assertion fault will result if you try.
case OP_IfNeg:					//	jump, in1
	pIn1 = &aMem[pOp.p1]
	assert( pIn1.flags.(int64) )
	if pIn1.Integer() < 0 {
		pc = pOp.p2 - 1
	}

//	Opcode: IfZero P1 P2 P3 * *
//	The register P1 must contain an integer. Add literal P3 to the value in register P1. If the result is exactly 0, jump to P2. 
//	It is illegal to use this instruction on a register that does not contain an integer. An assertion fault will result if you try.
case OP_IfZero:					//	jump, in1
	pIn1 = &aMem[pOp.p1]
	assert( pIn1.flags.(int64) )
	pIn1.AddInteger(pOp.p3)
	if pIn1.Integer() == 0 {
		pc = pOp.p2 - 1
	}

//	Opcode: AggStep * P2 P3 P4 P5
//	Execute the step function for an aggregate. The function has P5 arguments. P4 is a pointer to the FuncDef structure that specifies the function. Use register P3 as the accumulator.
//	The P5 arguments are taken from register P2 and its successors.
case OP_AggStep: {
	u.cf.n = pOp.p5
	assert( u.cf.n >= 0 )
	u.cf.pRec = &aMem[pOp.p2]
	u.cf.apVal = p.apArg
	assert( u.cf.apVal || u.cf.n == 0 )
	for(u.cf.i=0; u.cf.i<u.cf.n; u.cf.i++, u.cf.pRec++){
		assert( memIsValid(u.cf.pRec) )
		u.cf.apVal[u.cf.i] = u.cf.pRec
		memAboutToChange(p, u.cf.pRec)
		u.cf.pRec.StoreType()
	}
	u.cf.ctx.pFunc = pOp.p4.pFunc
	assert( pOp.p3 > 0 && pOp.p3 <= p.nMem )
	u.cf.pMem = &aMem[pOp.p3]
	u.cf.ctx.pMem = u.cf.pMem
	u.cf.pMem.n++
	u.cf.ctx.s.Value = nil
	u.cf.ctx.s.z = 0
	u.cf.ctx.s.zMalloc = 0
	u.cf.ctx.s.xDel = 0
	u.cf.ctx.s.db = db
	u.cf.ctx.isError = 0
	u.cf.ctx.pColl = 0
	u.cf.ctx.skipFlag = 0
	if u.cf.ctx.pFunc.flags & SQLITE_FUNC_NEEDCOLL  {
		assert( &pOp > &p.Program )
		assert( pOp[-1].p4type == P4_COLLSEQ )
		assert( pOp[-1].opcode == OP_CollSeq )
		u.cf.ctx.pColl = pOp[-1].p4.pColl
	}
	u.cf.ctx.pFunc.xStep(&u.cf.ctx, u.cf.n, u.cf.apVal)					//	IMP: R-24505-23230
	if u.cf.ctx.isError {
		p.zErrMsg = fmt.Sprintf("%v", sqlite3_value_text(&u.cf.ctx.s))
		rc = u.cf.ctx.isError
	}
	if u.cf.ctx.skipFlag {
		assert( pOp[-1].opcode == OP_CollSeq )
		if u.cf.i = pOp[-1].p1; u.cf.i != nil {
			aMem[u.cf.i].SetInt64(1)
		}
	}
	u.cf.ctx.s.Release()
}

//	Opcode: AggFinal P1 P2 * P4 *
//	Execute the finalizer function for an aggregate. P1 is the memory location that is the accumulator for the aggregate.
//	P2 is the number of arguments that the step function takes and P4 is a pointer to the FuncDef for this function. The P2 argument is not used by this opcode. It is only there to disambiguate functions that can take varying numbers of arguments. The P4 argument is only needed for the degenerate case where the step function was not previously called.
case OP_AggFinal:
	assert( pOp.p1 > 0 && pOp.p1 <= p.nMem )
	u.cg.pMem = &aMem[pOp.p1]
	assert( u.cg.pMem.flags & ~MEM_Agg == 0 )
	assert( u.cg.pMem.Value != nil )
	if rc = u.cg.pMem.Finalize(pOp.p4.pFunc); rc != SQLITE_OK {
		p.zErrMsg = sqlite3_value_text(u.cg.pMem)
	}
	sqlite3VdbeChangeEncoding(u.cg.pMem, encoding)
	UPDATE_MAX_BLOBSIZE(u.cg.pMem)
	if u.cg.pMem.VdbeMemTooBig() {
		goto too_big
	}

/* Opcode: Checkpoint P1 P2 P3 * *
**
** Checkpoint database P1. This is a no-op if P1 is not currently in
** WAL mode. Parameter P2 is one of SQLITE_CHECKPOINT_PASSIVE, FULL
** or RESTART.  Write 1 or 0 into mem[P3] if the checkpoint returns
** SQLITE_BUSY or not, respectively.  Write the number of pages in the
** WAL after the checkpoint into mem[P3+1] and the number of pages
** in the WAL that have been checkpointed after the checkpoint
** completes into mem[P3+2].  However on an error, mem[P3+1] and
** mem[P3+2] are initialized to -1.
*/
case OP_Checkpoint: {
  u.ch.aRes[0] = 0;
  u.ch.aRes[1] = u.ch.aRes[2] = -1;
  assert( pOp.p2==SQLITE_CHECKPOINT_PASSIVE
       || pOp.p2==SQLITE_CHECKPOINT_FULL
       || pOp.p2==SQLITE_CHECKPOINT_RESTART
  );
  rc = sqlite3Checkpoint(db, pOp.p1, pOp.p2, &u.ch.aRes[1], &u.ch.aRes[2]);
  if( rc==SQLITE_BUSY ){
    rc = SQLITE_OK;
    u.ch.aRes[0] = 1;
  }
  for(u.ch.i=0, u.ch.pMem = &aMem[pOp.p3]; u.ch.i<3; u.ch.i++, u.ch.pMem++){
    u.ch.pMem.SetInt64(int64(u.ch.aRes[u.ch.i]))
  }
  break;
};  

#ifndef SQLITE_OMIT_PRAGMA
/* Opcode: JournalMode P1 P2 P3 * P5
**
** Change the journal mode of database P1 to P3. P3 must be one of the
** PAGER_JOURNALMODE_XXX values. If changing between the various rollback
** modes (delete, truncate, persist, off and memory), this is a simple
** operation. No IO is required.
**
** If changing into or out of WAL mode the procedure is more complicated.
**
** Write a string containing the final journal-mode to register P2.
*/
case OP_JournalMode: {    /* out2-prerelease */
  u.ci.eNew = pOp.p3;
  assert( u.ci.eNew==PAGER_JOURNALMODE_DELETE
       || u.ci.eNew==PAGER_JOURNALMODE_TRUNCATE
       || u.ci.eNew==PAGER_JOURNALMODE_PERSIST
       || u.ci.eNew==PAGER_JOURNALMODE_OFF
       || u.ci.eNew==PAGER_JOURNALMODE_MEMORY
       || u.ci.eNew==PAGER_JOURNALMODE_WAL
       || u.ci.eNew==PAGER_JOURNALMODE_QUERY
  );
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )

  u.ci.pBt = db.Databases[pOp.p1].pBt;
  u.ci.pPager = u.ci.pBt.Pager()
  u.ci.eOld = sqlite3PagerGetJournalMode(u.ci.pPager);
  if( u.ci.eNew==PAGER_JOURNALMODE_QUERY ) u.ci.eNew = u.ci.eOld;
  if( !sqlite3PagerOkToChangeJournalMode(u.ci.pPager) ) u.ci.eNew = u.ci.eOld;

  u.ci.zFilename = sqlite3PagerFilename(u.ci.pPager);

  /* Do not allow a transition to journal_mode=WAL for a database
  ** in temporary storage or if the VFS does not support shared memory
  */
  if( u.ci.eNew==PAGER_JOURNALMODE_WAL
   && (sqlite3Strlen30(u.ci.zFilename)==0           /* Temp file */
       || !sqlite3PagerWalSupported(u.ci.pPager))   /* No shared-memory support */
  ){
    u.ci.eNew = u.ci.eOld;
  }

  if( (u.ci.eNew!=u.ci.eOld)
   && (u.ci.eOld==PAGER_JOURNALMODE_WAL || u.ci.eNew==PAGER_JOURNALMODE_WAL)
  ){
    if( !db.autoCommit || db.activeVdbeCnt>1 ){
      rc = SQLITE_ERROR;
      p.zErrMsg = fmt.Sprintf("cannot change %v wal mode from within a transaction", (u.ci.eNew == PAGER_JOURNALMODE_WAL ? "into" : "out of"));
      break;
    }else{

      if( u.ci.eOld==PAGER_JOURNALMODE_WAL ){
        /* If leaving WAL mode, close the log file. If successful, the call
        ** to PagerCloseWal() checkpoints and deletes the write-ahead-log
        ** file. An EXCLUSIVE lock may still be held on the database file
        ** after a successful return.
        */
        rc = sqlite3PagerCloseWal(u.ci.pPager);
        if( rc==SQLITE_OK ){
          sqlite3PagerSetJournalMode(u.ci.pPager, u.ci.eNew);
        }
      }else if( u.ci.eOld==PAGER_JOURNALMODE_MEMORY ){
        /* Cannot transition directly from MEMORY to WAL.  Use mode OFF
        ** as an intermediate */
        sqlite3PagerSetJournalMode(u.ci.pPager, PAGER_JOURNALMODE_OFF);
      }

      /* Open a transaction on the database file. Regardless of the journal
      ** mode, this transaction always uses a rollback journal.
      */
      assert( !u.ci.pBt.IsInTrans() )
      if( rc==SQLITE_OK ){
        rc = sqlite3BtreeSetVersion(u.ci.pBt, (u.ci.eNew==PAGER_JOURNALMODE_WAL ? 2 : 1));
      }
    }
  }

  if( rc ){
    u.ci.eNew = u.ci.eOld;
  }
  u.ci.eNew = sqlite3PagerSetJournalMode(u.ci.pPager, u.ci.eNew);

  pOut = &aMem[pOp.p2];
  pOut.flags = MEM_Str|MEM_Static|MEM_Term;
  pOut.z = JOURNAL_MODES(u.ci.eNew)
  pOut.n = sqlite3Strlen30(pOut.z);
  pOut.enc = SQLITE_UTF8;
  sqlite3VdbeChangeEncoding(pOut, encoding);
  break;
};
#endif /* SQLITE_OMIT_PRAGMA */

#if !defined(SQLITE_OMIT_VACUUM) && !defined(SQLITE_OMIT_ATTACH)
/* Opcode: Vacuum * * * * *
**
** Vacuum the entire database.  This opcode will cause other virtual
** machines to be created and run.  It may not be called from within
** a transaction.
*/
case OP_Vacuum: {
  rc = sqlite3RunVacuum(&p.zErrMsg, db);
  break;
}
#endif

/* Opcode: IncrVacuum P1 P2 * * *
**
** Perform a single step of the incremental vacuum procedure on
** the P1 database. If the vacuum has finished, jump to instruction
** P2. Otherwise, fall through to the next instruction.
*/
case OP_IncrVacuum: {        /* jump */
  assert( pOp.p1 >= 0 && pOp.p1 < len(db.Databases) )
  assert( (p.btreeMask & (((yDbMask)1)<<pOp.p1))!=0 );
  u.cj.pBt = db.Databases[pOp.p1].pBt;
  rc = sqlite3BtreeIncrVacuum(u.cj.pBt);
  if( rc==SQLITE_DONE ){
    pc = pOp.p2 - 1;
    rc = SQLITE_OK;
  }
  break;
}

/* Opcode: Expire P1 * * * *
**
** Cause precompiled statements to become expired. An expired statement
** fails with an error code of SQLITE_SCHEMA if it is ever executed 
** (via sqlite3_step()).
** 
** If P1 is 0, then all SQL statements become expired. If P1 is non-zero,
** then only the currently executing statement is affected. 
*/
case OP_Expire: {
  if( !pOp.p1 ){
    db.ExpirePreparedStatements()
  }else{
    p.expired = 1;
  }
  break;
}

/* Opcode: TableLock P1 P2 P3 P4 *
**
** Obtain a lock on a particular table. This instruction is only used when
** the shared-cache feature is enabled. 
**
** P1 is the index of the database in sqlite3.Databases[] of the database
** on which the lock is acquired.  A readlock is obtained if P3==0 or
** a write lock if P3==1.
**
** P2 contains the root-page of the table to lock.
**
** P4 contains a pointer to the name of the table being locked. This is only
** used to generate an error message if the lock cannot be obtained.
*/
case OP_TableLock: {
  byte isWriteLock = (byte)pOp.p3;
  if( isWriteLock || 0==(db.flags&SQLITE_ReadUncommitted) ){
    int p1 = pOp.p1; 
    assert( p1 >= 0 && p1 < len(db.Databases) )
    assert( (p.btreeMask & (((yDbMask)1)<<p1))!=0 );
    assert( isWriteLock==0 || isWriteLock==1 );
    rc = sqlite3BtreeLockTable(db.Databases[p1].pBt, pOp.p2, isWriteLock);
    if( (rc&0xFF)==SQLITE_LOCKED ){
      const char *z = pOp.p4.z;
      p.zErrMsg = fmt.Sprintf("database table is locked: %v", z);
    }
  }
  break;
}

/* Opcode: VBegin * * * P4 *
**
** P4 may be a pointer to an sqlite3_vtab structure. If so, call the 
** xBegin method for that table.
**
** Also, whether or not P4 is set, check that this is not being called from
** within a callback to a virtual table xSync() method. If it is, the error
** code will be set to SQLITE_LOCKED.
*/
case OP_VBegin: {
  u.ck.pVTab = pOp.p4.pVtab;
  rc = sqlite3VtabBegin(db, u.ck.pVTab);
  if( u.ck.pVTab ) importVtabErrMsg(p, u.ck.pVTab.pVtab);
  break;
}

/* Opcode: VCreate P1 * * P4 *
**
** P4 is the name of a virtual table in database P1. Call the xCreate method
** for that table.
*/
case OP_VCreate: {
  rc = sqlite3VtabCallCreate(db, pOp.p1, pOp.p4.z, &p.zErrMsg);
  break;
}

/* Opcode: VDestroy P1 * * P4 *
**
** P4 is the name of a virtual table in database P1.  Call the xDestroy method
** of that table.
*/
case OP_VDestroy: {
  p.inVtabMethod = 2;
  rc = db.VtabCallDestroy(pOp.p1, pOp.p4.z);
  p.inVtabMethod = 0;
  break;
}

/* Opcode: VOpen P1 * * P4 *
**
** P4 is a pointer to a virtual table object, an sqlite3_vtab structure.
** P1 is a cursor number.  This opcode opens a cursor to the virtual
** table and stores that cursor in P1.
*/
case OP_VOpen: {
  u.cl.pCur = 0;
  u.cl.pVtabCursor = 0;
  u.cl.pVtab = pOp.p4.pVtab.pVtab;
  u.cl.Callbacks = (sqlite3_module *)u.cl.pVtab.Callbacks;
  assert(u.cl.pVtab && u.cl.Callbacks);
  rc = u.cl.Callbacks.xOpen(u.cl.pVtab, &u.cl.pVtabCursor);
  importVtabErrMsg(p, u.cl.pVtab);
  if( SQLITE_OK==rc ){
    /* Initialize sqlite3_vtab_cursor base class */
    u.cl.pVtabCursor.pVtab = u.cl.pVtab;

    /* Initialise vdbe cursor object */
    u.cl.pCur = allocateCursor(p, pOp.p1, 0, -1, 0);
    if( u.cl.pCur ){
      u.cl.pCur.pVtabCursor = u.cl.pVtabCursor;
      u.cl.pCur.Callbacks = u.cl.pVtabCursor.pVtab.Callbacks;
    }else{
      db.mallocFailed = true
      u.cl.Callbacks.xClose(u.cl.pVtabCursor);
    }
  }
  break;
}

//	Opcode: VFilter P1 P2 P3 P4 *
//	P1 is a cursor opened using VOpen. P2 is an address to jump to if the filtered result set is empty.
//	P4 is either NULL or a string that was generated by the xBestIndex method of the module. The interpretation of the P4 string is left to the module implementation.
//	This opcode invokes the xFilter method on the virtual table specified by P1. The integer query plan parameter to xFilter is stored in register P3. Register P3 + 1 stores the argc parameter to be passed to the xFilter method. Registers P3 + 2..P3 + 1 + argc are the argc additional parameters which are passed to xFilter as argv. Register P3+2 becomes argv[0] when passed to xFilter.
//	A jump is made to P2 if the result set after filtering would be empty.
case OP_VFilter:				//	jump
	u.cm.pQuery = &aMem[pOp.p3]
	u.cm.pArgc = &u.cm.pQuery[1]
	u.cm.pCur = p.apCsr[pOp.p1]
	assert( memIsValid(u.cm.pQuery) )
	REGISTER_TRACE(pOp.p3, u.cm.pQuery)
	assert( u.cm.pCur.pVtabCursor )
	u.cm.pVtabCursor = u.cm.pCur.pVtabCursor
	u.cm.pVtab = u.cm.pVtabCursor.pVtab
	u.cm.Callbacks = u.cm.pVtab.Callbacks

	//	Grab the index number and argc parameters
	assert( u.cm.pQuery.Value.(int64) )
	assert( u.cm.pArgc.Value.(int64) )
	u.cm.nArg = int(u.cm.pArgc.Integer())
	u.cm.iQuery = int(u.cm.pQuery.Integer())

	//	Invoke the xFilter method
	u.cm.res = 0
	u.cm.apArg = p.apArg
	for u.cm.i = 0; u.cm.i < u.cm.nArg; u.cm.i++ {
		u.cm.apArg[u.cm.i] = &u.cm.pArgc[u.cm.i + 1]
		u.cm.apArg[u.cm.i].StoreType()
	}

	p.inVtabMethod = 1
	rc = u.cm.Callbacks.xFilter(u.cm.pVtabCursor, u.cm.iQuery, pOp.p4.z, u.cm.nArg, u.cm.apArg)
	p.inVtabMethod = 0
	importVtabErrMsg(p, u.cm.pVtab)
	if rc == SQLITE_OK {
		u.cm.res = u.cm.Callbacks.xEof(u.cm.pVtabCursor)
	}
	if u.cm.res {
		pc = pOp.p2 - 1
	}
	u.cm.pCur.nullRow = 0

//	Opcode: VColumn P1 P2 P3 * *
//	Store the value of the P2-th column of the row of the virtual-table that the P1 cursor is pointing to into register P3.
case OP_VColumn:
	pCur := p.apCsr[pOp.p1]
	assert( pCur.pVtabCursor )
	assert( pOp.p3 > 0 && pOp.p3 <= p.nMem )
	u.cn.pDest = &aMem[pOp.p3]
	memAboutToChange(p, u.cn.pDest)
	if pCur.nullRow {
		u.cn.pDest.SetNull()
		break
	}
	u.cn.pVtab = pCur.pVtabCursor.pVtab
	u.cn.Callbacks = u.cn.pVtab.Callbacks
	assert( u.cn.Callbacks.xColumn )
	memset(&u.cn.sContext, 0, sizeof(u.cn.sContext))

	//	The output cell may already have a buffer allocated. Move the current contents to u.cn.sContext.s so in case the user-function can use the already allocated buffer instead of allocating a new one.
	sqlite3VdbeMemMove(&u.cn.sContext.s, u.cn.pDest)
	u.cn.sContext.s.Value = nil

	rc = u.cn.Callbacks.xColumn(pCur.pVtabCursor, &u.cn.sContext, pOp.p2)
	importVtabErrMsg(p, u.cn.pVtab)
	if u.cn.sContext.isError {
		rc = u.cn.sContext.isError
	}

	//	Copy the result of the function to the P3 register. We do this regardless of whether or not an error occurred to ensure any dynamic allocation in u.cn.sContext.s (a Mem struct) is released.
	sqlite3VdbeChangeEncoding(&u.cn.sContext.s, encoding)
	sqlite3VdbeMemMove(u.cn.pDest, &u.cn.sContext.s)
	REGISTER_TRACE(pOp.p3, u.cn.pDest)
	UPDATE_MAX_BLOBSIZE(u.cn.pDest)

	if u.cn.pDest.VdbeMemTooBig() {
		goto too_big
	}

//	Opcode: VNext P1 P2 * * *
//	Advance virtual table P1 to the next row in its result set and jump to instruction P2. Or, if the virtual table has reached the end of its result set, then fall through to the next instruction.
case OP_VNext:					//	jump
	u.co.res = 0
	u.co.pCur = p.apCsr[pOp.p1]
	assert( u.co.pCur.pVtabCursor != nil )
	if !u.co.pCur.nullRow {
		u.co.pVtab = u.co.pCur.pVtabCursor.pVtab
		u.co.Callbacks = u.co.pVtab.Callbacks
		assert( u.co.Callbacks.xNext )

		//	Invoke the xNext() method of the module. There is no way for the underlying implementation to return an error if one occurs during xNext(). Instead, if an error occurs, true is returned (indicating that data is available) and the error code returned when xColumn or some other method is next invoked on the save virtual table cursor.
		p.inVtabMethod = 1
		rc = u.co.Callbacks.xNext(u.co.pCur.pVtabCursor)
		p.inVtabMethod = 0
		importVtabErrMsg(p, u.co.pVtab)
		if rc == SQLITE_OK {
			u.co.res = u.co.Callbacks.xEof(u.co.pCur.pVtabCursor)
		}
		if !u.co.res {
			//	If there is data, jump to P2
			pc = pOp.p2 - 1
		}
	}

//	Opcode: VRename P1 * * P4 *
//	P4 is a pointer to a virtual table object, an sqlite3_vtab structure. This opcode invokes the corresponding xRename method. The value in register P1 is passed as the Name argument to the xRename method.
case OP_VRename:
	u.cp.pVtab = pOp.p4.pVtab.pVtab
	u.cp.pName = &aMem[pOp.p1]
	assert( u.cp.pVtab.Callbacks.xRename )
	assert( memIsValid(u.cp.pName) )
	REGISTER_TRACE(pOp.p1, u.cp.pName)
	assert( u.cp.pName.flags & MEM_Str )
	if rc = sqlite3VdbeChangeEncoding(u.cp.pName, SQLITE_UTF8); rc == SQLITE_OK {
		rc = u.cp.pVtab.Callbacks.xRename(u.cp.pVtab, u.cp.pName.z)
		importVtabErrMsg(p, u.cp.pVtab)
		p.expired = 0
	}

/* Opcode: VUpdate P1 P2 P3 P4 *
**
** P4 is a pointer to a virtual table object, an sqlite3_vtab structure.
** This opcode invokes the corresponding xUpdate method. P2 values
** are contiguous memory cells starting at P3 to pass to the xUpdate 
** invocation. The value in register (P3+P2-1) corresponds to the 
** p2th element of the argv array passed to xUpdate.
**
** The xUpdate method will do a DELETE or an INSERT or both.
** The argv[0] element (which corresponds to memory cell P3)
** is the rowid of a row to delete.  If argv[0] is NULL then no 
** deletion occurs.  The argv[1] element is the rowid of the new 
** row.  This can be NULL to have the virtual table select the new 
** rowid for itself.  The subsequent elements in the array are 
** the values of columns in the new row.
**
** If P2==1 then no insert is performed.  argv[0] is the rowid of
** a row to delete.
**
** P1 is a boolean flag. If it is set to true and the xUpdate call
** is successful, then the value returned by sqlite3_last_insert_rowid() 
** is set to the value of the rowid for the row just inserted.
*/
case OP_VUpdate: {
  assert( pOp.p2==1        || pOp.p5==OE_Fail   || pOp.p5==OE_Rollback
       || pOp.p5==OE_Abort || pOp.p5==OE_Ignore || pOp.p5==OE_Replace
  );
  u.cq.pVtab = pOp.p4.pVtab.pVtab;
  u.cq.Callbacks = (sqlite3_module *)u.cq.pVtab.Callbacks;
  u.cq.nArg = pOp.p2;
  assert( pOp.p4type==P4_VTAB );
  if( u.cq.Callbacks.xUpdate ){
    byte vtabOnConflict = db.vtabOnConflict;
    u.cq.apArg = p.apArg;
    u.cq.pX = &aMem[pOp.p3];
    for(u.cq.i=0; u.cq.i<u.cq.nArg; u.cq.i++){
      assert( memIsValid(u.cq.pX) );
      memAboutToChange(p, u.cq.pX);
      u.cq.pX.StoreType()
      u.cq.apArg[u.cq.i] = u.cq.pX;
      u.cq.pX++;
    }
    db.vtabOnConflict = pOp.p5;
    rc = u.cq.Callbacks.xUpdate(u.cq.pVtab, u.cq.nArg, u.cq.apArg, &u.cq.rowid);
    db.vtabOnConflict = vtabOnConflict;
    importVtabErrMsg(p, u.cq.pVtab);
    if( rc==SQLITE_OK && pOp.p1 ){
      assert( u.cq.nArg > 1 && u.cq.apArg[0] && u.cq.apArg[0].Value == nil )
      db.lastRowid = lastRowid = u.cq.rowid;
    }
    if( rc==SQLITE_CONSTRAINT && pOp.p4.pVtab.SupportsConstraints ){
      if( pOp.p5==OE_Ignore ){
        rc = SQLITE_OK;
      }else{
        p.errorAction = ((pOp.p5==OE_Replace) ? OE_Abort : pOp.p5);
      }
    }else{
      p.nChange++;
    }
  }
  break;
}

#ifndef  SQLITE_OMIT_PAGER_PRAGMAS
//	Opcode: Pagecount P1 P2 * * *
//	Write the current number of pages in database P1 to memory cell P2.
case OP_Pagecount:				// out2-prerelease
	pOut.Store(sqlite3BtreeLastPage(db.Databases[pOp.p1].pBt))
#endif


#ifndef  SQLITE_OMIT_PAGER_PRAGMAS
//	Opcode: MaxPgcnt P1 P2 P3 * *
//	Try to set the maximum page count for database P1 to the value in P3. Do not let the maximum page count fall below the current page count and do not change the maximum page count value if P3 == 0.
//	Store the maximum page count after the change in register P2.
case OP_MaxPgcnt:				//	out2-prerelease
	var newMax	uint
	pBt := db.Databases[pOp.p1].pBt
	if pOp.p3 != 0 {
		newMax = sqlite3BtreeLastPage(pBt)
		if newMax < uint(pOp.p3) {
			newMax = uint(pOp.p3)
		}
	}
	pOut.Store(sqlite3BtreeMaxPageCount(pBt, newMax))
#endif


#ifndef SQLITE_OMIT_TRACE
//	Opcode: Trace * * * P4 *
//	If tracing is enabled (by the sqlite3_trace()) interface, then the UTF-8 string contained in P4 is emitted on the trace callback.
case OP_Trace:
	if db.xTrace {
		if pOp.p4.z != "" {
			u.cr.zTrace = pOp.p4.z
		} else {
			u.cr.zTrace = p.zSql
		}
		if u.cr.zTrace != 0 {
			u.cr.z = p.ExpandSql(u.cr.zTrace)
			db.xTrace(db.pTraceArg, u.cr.z)
			u.cr.z = nil
		}
	}
#endif


//	Opcode: Noop * * * * *
//	Do nothing. This instruction is often useful as a jump destination.
//	The magic Explain opcode are only inserted when explain == 2 (which is to say when the EXPLAIN QUERY PLAN syntax is used.) This opcode records information from the optimizer. It is the the same as a no-op. This opcode never appears in a real VM program.
default:		//	This is really OP_Noop and OP_Explain
	assert( pOp.opcode == OP_Noop || pOp.opcode == OP_Explain )

/*****************************************************************************
** The cases of the switch statement above this line should all be indented
** by 6 spaces.  But the left-most 6 spaces have been removed to improve the
** readability.  From this point on down, the normal indentation rules are
** restored.
*****************************************************************************/
}

#ifdef VDBE_PROFILE
    {
      uint64 elapsed = sqlite3Hwtime() - start;
      pOp.cycles += elapsed;
      pOp.cnt++;
    }
#endif

  }  /* The end of the for(;;) loop the loops through opcodes */

  /* If we reach this point, it means that execution is finished with
  ** an error of some kind.
  */
vdbe_error_halt:
  assert( rc );
  p.rc = rc;
  sqlite3_log(rc, "statement aborts at %d: [%s] %s", pc, p.zSql, p.zErrMsg);
  p.Halt()
  if( rc==SQLITE_IOERR_NOMEM ) db.mallocFailed = 1;
  rc = SQLITE_ERROR;
  if( resetSchemaOnFault>0 ){
    db.ResetInternalSchema(resetSchemaOnFault - 1)
  }

  /* This is the only way out of this procedure.  We have to
  ** release the mutexes on btrees that were acquired at the
  ** top. */
vdbe_return:
  db.lastRowid = lastRowid;
  p.Leave()
  return rc;

  /* Jump to here if a string or blob larger than SQLITE_MAX_LENGTH
  ** is encountered.
  */
too_big:
  p.zErrMsg = "string or blob too big";
  rc = SQLITE_TOOBIG;
  goto vdbe_error_halt;

  /* Jump to here if a malloc() fails.
  */
no_mem:
  db.mallocFailed = true
  p.zErrMsg = "out of memory";
  rc = SQLITE_NOMEM;
  goto vdbe_error_halt;

  /* Jump to here for any other kind of fatal error.  The "rc" variable
  ** should hold the error number.
  */
abort_due_to_error:
  assert( p.zErrMsg==0 );
  if( db.mallocFailed ) rc = SQLITE_NOMEM;
  if( rc!=SQLITE_IOERR_NOMEM ){
    p.zErrMsg = fmt.Sprintf("%v", sqlite3ErrStr(rc));
  }
  goto vdbe_error_halt;

  /* Jump to here if the sqlite3_interrupt() API sets the interrupt
  ** flag.
  */
abort_due_to_interrupt:
  assert( db.u1.isInterrupted );
  rc = SQLITE_INTERRUPT;
  p.rc = rc;
  p.zErrMsg = fmt.Sprintf("%v", sqlite3ErrStr(rc));
  goto vdbe_error_halt;
}
