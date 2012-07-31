/* This module implements the sqlite3_status() interface and related
** functionality.
*/
/************** Include vdbeInt.h in the middle of status.c ******************/
/************** Begin file vdbeInt.h *****************************************/
/* This is the header file for information that is private to the
** VDBE.  This information used to all be at the top of the single
** source code file "vdbe.c".  When that file became too big (over
** 6000 lines long) it was split up into several smaller files and
** this header information was factored out.
*/
#ifndef _VDBEINT_H_
#define _VDBEINT_H_

/*
** SQL is translated into a sequence of instructions to be
** executed by a virtual machine.  Each instruction is an instance
** of the following structure.
*/
typedef struct VdbeOp Op;

/*
** Boolean values
*/
typedef unsigned char Bool;

/* Opaque type used by code in vdbesort.c */
typedef struct VdbeSorter VdbeSorter;

/* Opaque type used by the explainer */
typedef struct Explain Explain;

/*
** A cursor is a pointer into a single BTree within a database file.
** The cursor can seek to a BTree entry with a particular key, or
** loop over all entries of the Btree.  You can also insert new BTree
** entries or retrieve the key or data from the entry that the cursor
** is currently pointing to.
** 
** Every cursor that the virtual machine has open is represented by an
** instance of the following structure.
*/
struct VdbeCursor {
  btree.Cursor *pCursor;    /* The cursor structure of the backend */
  Btree *pBt;           /* Separate file holding temporary table */
  KeyInfo *pKeyInfo;    /* Info about index keys needed by index cursors */
  int iDb;              /* Index of cursor database in db.Databases[] (or -1) */
  int pseudoTableReg;   /* Register holding pseudotable content. */
  int nField;           /* Number of fields in the header */
  Bool zeroed;          /* True if zeroed out and ready for reuse */
  Bool rowidIsValid;    /* True if lastRowid is valid */
  Bool atFirst;         /* True if pointing to first entry */
  Bool useRandomRowid;  /* Generate new record numbers semi-randomly */
  Bool nullRow;         /* True if pointing to a row with no data */
  Bool deferredMoveto;  /* A call to sqlite3BtreeMoveto() is needed */
  Bool isTable;         /* True if a table requiring integer keys */
  Bool isIndex;         /* True if an index containing keys only - no data */
  Bool isOrdered;       /* True if the underlying table is BTREE_UNORDERED */
  Bool isSorter;        /* True if a new-style sorter */
  sqlite3_vtab_cursor *pVtabCursor;  /* The cursor for a virtual table */
  const sqlite3_module *pModule;     /* Module for cursor pVtabCursor */
  int64 seqCount;         /* Sequence counter */
  int64 movetoTarget;     /* Argument to the deferred sqlite3BtreeMoveto() */
  int64 lastRowid;        /* Last rowid from a Next or NextIdx operation */
  VdbeSorter *pSorter;  /* Sorter object for OP_SorterOpen cursors */

  /* Result of last sqlite3BtreeMoveto() done by an OP_NotExists or 
  ** OP_IsUnique opcode on this cursor. */
  int seekResult;

  /* Cached information about the header for the data record that the
  ** cursor is currently pointing to.  Only valid if cacheStatus matches
  ** Vdbe.cacheCtr.  Vdbe.cacheCtr will never take on the value of
  ** CACHE_STALE and so setting cacheStatus=CACHE_STALE guarantees that
  ** the cache is out of date.
  **
  ** aRow might point to (ephemeral) data for the current row, or it might
  ** be NULL.
  */
  uint32 cacheStatus;      /* Cache is valid if this matches Vdbe.cacheCtr */
  int payloadSize;      /* Total number of bytes in the record */
  uint32 *aType;           /* Type values for all entries in the record */
  uint32 *aOffset;         /* Cached offsets to the start of each columns data */
  byte *aRow;             /* Data for the current row, if all on one page */
};
typedef struct VdbeCursor VdbeCursor;

//	When a sub-program is executed (OP_Program), a structure of this type is allocated to store the current value of the program counter, as well as the current memory cell array and various other frame specific values stored in the Vdbe struct. When the sub-program is finished, these values are copied back to the Vdbe from the VdbeFrame structure, restoring the state of the VM to as it was before the sub-program began executing.
//	The memory for a VdbeFrame object is allocated and managed by a memory cell in the parent (calling) frame. When the memory cell is deleted or overwritten, the VdbeFrame object is not freed immediately. Instead, it is linked into the Vdbe.pDelFrame list. The contents of the Vdbe.pDelFrame list is deleted when the VM is reset in VdbeHalt(). The reason for doing this instead of deleting the VdbeFrame immediately is to avoid recursive calls to Mem::Release() when the memory cells belonging to the child frame are released.
//	The currently executing frame is stored in Vdbe.pFrame. Vdbe.pFrame is set to NULL if the currently executing frame is the main program.
typedef struct VdbeFrame VdbeFrame;
struct VdbeFrame {
  Vdbe *v;                /* VM this frame belongs to */
  VdbeFrame *pParent;     /* Parent of this frame, or NULL if parent is main */
  Program			[]*Op
  Mem *aMem;              /* Array of memory cells for parent frame */
  byte *aOnceFlag;          /* Array of OP_Once flags for parent frame */
  VdbeCursor **apCsr;     /* Array of Vdbe cursors for parent frame */
  void *token;            /* Copy of SubProgram.token */
  int64 lastRowid;          /* Last insert rowid (sqlite3.lastRowid) */
  uint16 nCursor;            /* Number of entries in apCsr */
  int pc;                 /* Program Counter in parent (calling) frame */
  int nMem;               /* Number of entries in aMem */
  int nOnceFlag;          /* Number of entries in aOnceFlag */
  int nChildMem;          /* Number of memory cells for child frame */
  int nChildCsr;          /* Number of cursors for child frame */
  int nChange;            /* Statement changes (Vdbe.nChanges)     */
};

#define VdbeFrameMem(p) ((Mem *)&((byte *)p)[ROUND(sizeof(VdbeFrame), 8)])

//	A value for VdbeCursor.cacheValid that means the cache is always invalid.
#define CACHE_STALE 0

type Zeroes		int

func (z Zeroes) ByteSlice() []byte {
	return make([]byte, z, z)
}

type BLOB		[]byte

func NewBLOB(v interface{}) (b BLOB) {
	b = BLOB(raw.ByteSlice(v))
}

func (b *BLOB) Store(v... interface{}) {
	(*b) = make(BLOB)
	b.Append(v...)
}

func (b *BLOB) Append(v... interface{}) {
	for _, x := range v {
		(*b) = append(b, BLOB(raw.ByteSlice(x)))
	}
}


//	Internally, the vdbe manipulates nearly all SQL values as Mem structures. Each Mem struct may cache multiple representations (string, integer etc.) of the same value.
struct Mem {
	db		*sqlite3						//	The associated database connection
	z		string							//	String or BLOB value
	r		float64							//	Real value
	Value	interface{}
//	union {
//		int64 i;              /* Integer value used when  is set in flags */
//		FuncDef *pDef;      /* Used only when flags==MEM_Agg */
//		RowSet *pRowSet;    /* Used only when flags==MEM_RowSet */
//		VdbeFrame *pFrame;  /* Used when flags==MEM_Frame */
//	} u;
	n		int;							//	Number of characters in string value, excluding '\0'
	flags	uint16							//	Some combination of MEM_Null, MEM_Str, MEM_Dyn, etc.
	type	byte							//	One of SQLITE_NULL, SQLITE_TEXT, SQLITE_INTEGER, etc
	enc		byte							//	SQLITE_UTF8
	xDel	func(interface{}) interface{}	//	If not null, call this function to delete Mem.z
	zMalloc	[]byte							//	Dynamic buffer allocated by sqlite3_malloc()
}

func (p *Mem) Store(v interface{}) {
	switch x := v.(type); {
	case nil, int64:
		p.Value = v
	case int8:
		p.Value = int64(x)
	case int16:
		p.Value = int64(x)
	case int32:
		p.Value = int64(x)
	case uint8:
		p.Value = int64(x)
	case uint16:
		p.Value = int64(x)
	case uint32:
		p.Value = int64(x)
	case uint64:
		p.Value = int64(x)
	case uintptr:
		p.Value = int64(x)
	default:
		p.Value = NewBLOB(v)
	}
}

func (p *Mem) Integer() int {
	return p.Value.(int64)
}

func (p *Mem) AddInteger(x int64) {
	p.Value = p.Integer() + x
}

func (p *Mem) ByteLen() (l int) {
	l = len(([]byte)(p.z))
	if n, ok := p.Value.(Zeroes); ok {
		l += int(n)
	}
	return
}

func (pMem *Mem) IsBLOB() bool {
	switch pMem.Value.(type) {
	case nil, int64:
		return false
	}
	return true
}

//	If the given Mem* has a zero-filled tail, turn it into an ordinary blob stored in dynamically allocated space.
func (pMem *Mem) ExpandBlob() int {
	if v, ok := pMem.Value.(Zeroes); ok {
		assert( pMem.flags & MEM_Blob != 0 )
		assert( pMem.flags & MEM_RowSet == 0 )

		//	Set nByte to the number of bytes required to store the expanded blob.
		nByte := pMem.ByteLen()
		if nByte <= 0 {
			nByte = 1
		}
		if sqlite3VdbeMemGrow(pMem, nByte, 1) == SQLITE_OK {
			memset(&pMem.z[pMem.n], 0, int(v))
			pMem.n = pMem.ByteLen()
			pMem.Value = nil
			pMem.flags &= ~MEM_Term
		} else {
			rc = SQLITE_NOMEM
		}
	}
	return
}

//	This function is only available internally, it is not part of the external API. It works in a similar way to Mem_text(), except the data returned is in the encoding specified by the second parameter, which must be SQLITE_UTF8.
func (pVal *Mem) ValueText(enc byte) (s string) {
	if pVal != nil {
		assert( pVal.flags & MEM_RowSet == 0 )
		if pVal.Value != nil {
//			pVal.flags |= (pVal.flags & MEM_Blob) >> 3
			ExpandBlob(pVal)
			if pVal.flags & MEM_Str != 0 {
				sqlite3VdbeChangeEncoding(pVal, enc)
				sqlite3VdbeMemNulTerminate(pVal)				//	IMP: R-31275-44060
			} else {
				pVal.Stringify(enc)
				assert( 0==(1&SQLITE_PTR_TO_INT(pVal.z)) )
			}
			assert(pVal.enc == enc || pVal.db == 0 || pVal.db.mallocFailed )
			if pVal.enc == enc {
				s = pVal.z
			}
	    }
	}
	return
}


/* One or more of the following flags are set to indicate the validOK
** representations of the value stored in the Mem struct.
**
** If the MEM_Null flag is set, then the value is an SQL NULL value.
** No other flags may be set in this case.
**
** If the MEM_Str flag is set then Mem.z points at a string representation.
** Usually this is encoded in the same unicode encoding as the main
** database (see below for exceptions). If the MEM_Term flag is also
** set, then the string is nul terminated. The  and MEM_Real 
** flags may coexist with the MEM_Str flag.
*/
#define MEM_Null      0x0001   /* Value is NULL */
#define MEM_Str       0x0002   /* Value is a string */

#define MEM_Real      0x0008   /* Value is a real number */
#define MEM_Blob      0x0010   /* Value is a BLOB */
#define MEM_RowSet    0x0020   /* Value is a RowSet object */
#define MEM_Frame     0x0040   /* Value is a VdbeFrame object */
#define MEM_Invalid   0x0080   /* Value is undefined */
#define MEM_TypeMask  0x00ff   /* Mask of type bits */

/* Whenever Mem contains a valid string or blob representation, one of
** the following flags must be set to determine the memory management
** policy for Mem.z.  The MEM_Term flag tells us whether or not the
** string is \000 or \u0000 terminated
*/
#define MEM_Term      0x0200   /* String rep is nul terminated */
#define MEM_Dyn       0x0400   /* Need to call sqliteFree() on Mem.z */
#define MEM_Static    0x0800   /* Mem.z points to a static string */
#define MEM_Ephem     0x1000   /* Mem.z points to an ephemeral string */
#define MEM_Agg       0x2000   /* Mem.z points to an agg function context */

//	Clear any existing type flags from a Mem and replace them with f
func (p *Mem) SetTypeFlag(f int) {
	p.flags = (p.flags & ~MEM_TypeMask) | f
}


/* A VdbeFunc is just a FuncDef (defined in sqliteInt.h) that contains
** additional information about auxiliary information bound to arguments
** of the function.  This is used to implement the sqlite3_get_auxdata()
** and sqlite3_set_auxdata() APIs.  The "auxdata" is some auxiliary data
** that can be associated with a constant argument to a function.  This
** allows functions such as "regexp" to compile their constant regular
** expression argument once and reused the compiled code for multiple
** invocations.
*/
struct VdbeFunc {
  FuncDef *pFunc;               /* The definition of the function */
  int nAux;                     /* Number of entries allocated for apAux[] */
  struct AuxData {
    void *pAux;                   /* Aux data for the i-th argument */
    void (*xDelete)(void *);      /* Destructor for the aux data */
  } apAux[1];                   /* One slot for each function argument */
};

/*
** The "context" argument for a installable function.  A pointer to an
** instance of this structure is the first argument to the routines used
** implement the SQL functions.
**
** There is a typedef for this structure in sqlite.h.  So all routines,
** even the public interface to SQLite, can use a pointer to this structure.
** But this file is the only place where the internal details of this
** structure are known.
**
** This structure is defined inside of vdbeInt.h because it uses substructures
** (Mem) which are only defined there.
*/
struct sqlite3_context {
  FuncDef *pFunc;       /* Pointer to function information.  MUST BE FIRST */
  VdbeFunc *pVdbeFunc;  /* Auxilary data, if created. */
  Mem s;                /* The return value is stored here */
  Mem *pMem;            /* Memory cell used to store aggregate context */
  CollSeq *pColl;       /* Collating sequence */
  int isError;          /* Error code returned by the function. */
  int skipFlag;         /* Skip skip accumulator loading if true */
};

/*
** An Explain object accumulates indented output which is helpful
** in describing recursive data structures.
*/
struct Explain {
	pVdbe		*Vdbe		//	Attach the explanation to this Vdbe
	str			string		//	The string being accumulated
	nIndent		int			//	Number of elements in aIndent
	aIndent		[]uint16	//	Levels of indentation
}

/*
** An instance of the virtual machine.  This structure contains the complete
** state of the virtual machine.
**
** The "sqlite3_stmt" structure pointer that is returned by Prepare()
** is really a pointer to an instance of this structure.
**
** The Vdbe.inVtabMethod variable is set to non-zero for the duration of
** any virtual table method invocations made by the vdbe program. It is
** set to 2 for xDestroy method calls and 1 for all other methods. This
** variable is used for two purposes: to allow xDestroy methods to execute
** "DROP TABLE" statements and to prevent some nasty side effects of
** malloc failure when SQLite is invoked recursively by a virtual table 
** method function.
*/
struct Vdbe {
  sqlite3 *db;            /* The database connection that owns this statement */

	Program		[]*Op		//	Space to hold the virtual machine's program */

  Mem *aMem;              /* The memory locations */
  Mem **apArg;            /* Arguments to currently executing user function */
  Mem *aColName;          /* Column names to return */
  Mem *pResultSet;        /* Pointer to an array of results */
  int nMem;               /* Number of memory locations currently allocated */

  int nLabel;             /* Number of labels used */
  int *aLabel;            /* Space to hold the labels */
  uint16 nResColumn;         /* Number of columns in one row of the result set */
  uint16 nCursor;            /* Number of slots in apCsr[] */
  uint32 magic;              /* Magic number for sanity checking */
  char *zErrMsg;          /* Error message written here */
  Vdbe *pPrev,*Next;     /* Linked list of VDBEs with the same Vdbe.db */
  VdbeCursor **apCsr;     /* One element of this array for each open cursor */
  Mem *aVar;              /* Values for the OP_Variable opcode. */
  char **azVar;           /* Name of variables */
  ynVar nVar;             /* Number of entries in aVar[] */
  ynVar nzVar;            /* Number of entries in azVar[] */
  uint32 cacheCtr;           /* VdbeCursor row cache generation counter */
  int pc;                 /* The program counter */
  int rc;                 /* Value to return */
  byte errorAction;         /* Recovery action to do in case of an error */
  byte explain;             /* True if EXPLAIN present on SQL command */
  byte changeCntOn;         /* True to update the change-counter */
  byte expired;             /* True if the VM needs to be recompiled */
  byte runOnlyOnce;         /* Automatically expire on reset */
  byte minWriteFileFormat;  /* Minimum file format for writable database files */
  byte inVtabMethod;        /* See comments above */
  byte usesStmtJournal;     /* True if uses a statement journal */
  byte readOnly;            /* True for read-only statements */
  byte isPrepareV2;         /* True if prepared with prepare_v2() */
  int nChange;            /* Number of db changes made since last reset */
  yDbMask btreeMask;      /* Bitmask of db.Databases[] entries referenced */
  yDbMask lockMask;       /* Subset of btreeMask that requires a lock */
  int iStatement;         /* Statement number (or 0 if has not opened stmt) */
  int aCounter[3];        /* Counters used by sqlite3_stmt_status() */
#ifndef SQLITE_OMIT_TRACE
  int64 startTime;          /* Time when query started - used for profiling */
#endif
  int64 nFkConstraint;      /* Number of imm. FK constraints this VM */
  int64 nStmtDefCons;       /* Number of def. constraints when stmt started */
  char *zSql;             /* Text of the SQL statement that generated this */
  void *pFree;            /* Free this when deleting the vdbe */
  VdbeFrame *pFrame;      /* Parent frame */
  VdbeFrame *pDelFrame;   /* List of frame objects to free on VM reset */
  int nFrame;             /* Number of frames in pFrame list */
  uint32 expmask;            /* Binding to these vars invalidates VM */
  Routines			[]*SubProgram
  int nOnceFlag;          /* Size of array aOnceFlag[] */
  byte *aOnceFlag;          /* Flags for OP_Once */
};

/*
** The following are allowed values for Vdbe.magic
*/
#define VDBE_MAGIC_INIT     0x26bceaa5    /* Building a VDBE program */
#define VDBE_MAGIC_RUN      0xbdf20da3    /* VDBE is ready to execute */
#define VDBE_MAGIC_HALT     0x519c2973    /* VDBE has completed execution */
#define VDBE_MAGIC_DEAD     0xb606c3c8    /* The VDBE has been deallocated */

/*
** Function prototypes
*/
#if defined(VDBE_PROFILE)
 void sqlite3VdbePrintOp(FILE*, int, Op*);
#endif

func VdbeMemRelease(p *Mem) {
	if p.flags & (MEM_Agg | MEM_Dyn | MEM_RowSet | MEM_Frame) != 0 {
		p.ReleaseExternal()
	}
}

func ExpandBlob(p *Mem) (blob *Mem) {
	if v, ok := p.Value.(Zeroes); ok {
		blob = v.ExpandBlob()
	}
	return
}

#endif /* !defined(_VDBEINT_H_) */

/************** End of vdbeInt.h *********************************************/
/************** Continuing where we left off in status.c *********************/

/*
** Variables in which to record status information.
*/
typedef struct sqlite3StatType sqlite3StatType;
static struct sqlite3StatType {
  int nowValue[10];         /* Current value */
  int mxValue[10];          /* Maximum value */
} sqlite3Stat = { {0,}, {0,} };


/* The "wsdStat" macro will resolve to the status information
** state vector.  If writable static data is unsupported on the target,
** we have to locate the state vector at run-time.  In the more common
** case where writable static data is supported, wsdStat can refer directly
** to the "sqlite3Stat" state vector declared above.
*/
# define wsdStatInit
# define wsdStat sqlite3Stat

/*
** Return the current value of a status parameter.
*/
 int sqlite3StatusValue(int op){
  wsdStatInit;
  assert( op>=0 && op<ArraySize(wsdStat.nowValue) );
  return wsdStat.nowValue[op];
}

/*
** Add N to the value of a status record.  It is assumed that the
** caller holds appropriate locks.
*/
 void sqlite3StatusAdd(int op, int N){
  wsdStatInit;
  assert( op>=0 && op<ArraySize(wsdStat.nowValue) );
  wsdStat.nowValue[op] += N;
  if( wsdStat.nowValue[op]>wsdStat.mxValue[op] ){
    wsdStat.mxValue[op] = wsdStat.nowValue[op];
  }
}

/*
** Set the value of a status to X.
*/
 void sqlite3StatusSet(int op, int X){
  wsdStatInit;
  assert( op>=0 && op<ArraySize(wsdStat.nowValue) );
  wsdStat.nowValue[op] = X;
  if( wsdStat.nowValue[op]>wsdStat.mxValue[op] ){
    wsdStat.mxValue[op] = wsdStat.nowValue[op];
  }
}

/*
** Query status information.
**
** This implementation assumes that reading or writing an aligned
** 32-bit integer is an atomic operation.  If that assumption is not true,
** then this routine is not threadsafe.
*/
func int sqlite3_status(int op, int *pCurrent, int *pHighwater, int resetFlag){
  wsdStatInit;
  if( op<0 || op>=ArraySize(wsdStat.nowValue) ){
    return SQLITE_MISUSE_BKPT;
  }
  *pCurrent = wsdStat.nowValue[op];
  *pHighwater = wsdStat.mxValue[op];
  if( resetFlag ){
    wsdStat.mxValue[op] = wsdStat.nowValue[op];
  }
  return SQLITE_OK;
}

/*
** Query status information for a single database connection
*/
func int sqlite3_db_status(
  sqlite3 *db,          /* The database connection whose status is desired */
  int op,               /* Status verb */
  int *pCurrent,        /* Write current value here */
  int *pHighwater,      /* Write high-water mark here */
  int resetFlag         /* Reset high-water mark if true */
){
  int rc = SQLITE_OK;   /* Return code */
  db.mutex.Lock()
  switch( op ){
    case SQLITE_DBSTATUS_LOOKASIDE_USED: {
      *pCurrent = db.lookaside.nOut;
      *pHighwater = db.lookaside.mxOut;
      if( resetFlag ){
        db.lookaside.mxOut = db.lookaside.nOut;
      }
      break;
    }

    case SQLITE_DBSTATUS_LOOKASIDE_HIT:
    case SQLITE_DBSTATUS_LOOKASIDE_MISS_SIZE:
    case SQLITE_DBSTATUS_LOOKASIDE_MISS_FULL: {
      assert( (op-SQLITE_DBSTATUS_LOOKASIDE_HIT)>=0 );
      assert( (op-SQLITE_DBSTATUS_LOOKASIDE_HIT)<3 );
      *pCurrent = 0;
      *pHighwater = db.lookaside.anStat[op - SQLITE_DBSTATUS_LOOKASIDE_HIT];
      if( resetFlag ){
        db.lookaside.anStat[op - SQLITE_DBSTATUS_LOOKASIDE_HIT] = 0;
      }
      break;
    }

    /* 
    ** Return an approximation for the amount of memory currently used
    ** by all pagers associated with the given database connection.  The
    ** highwater mark is meaningless and is returned as zero.
    */
    case SQLITE_DBSTATUS_CACHE_USED: {
      int totalUsed = 0;
      int i;
      db.LockAll()
	  for _, database := range db.Databases {
        if pBt := database.pBt; pBt != nil {
          pPager := pBt.Pager()
          totalUsed += sqlite3PagerMemUsed(pPager)
        }
      }
      db.LeaveBtreeAll()
      *pCurrent = totalUsed;
      *pHighwater = 0;
      break;
    }

	//	*pCurrent gets an accurate estimate of the amount of memory used to store the schema for all databases (main, temp, and any ATTACHed databases. *pHighwater is set to zero.
/*    case SQLITE_DBSTATUS_SCHEMA_USED: {
      int i;                      // Used to iterate through schemas
      int nByte = 0;              // Used to accumulate return value

      db.LockAll()
      db.pnBytesFreed = &nByte;
	  for i, database := range db.Databases {
        if schema := database.Schema; schema != nil {
          nByte += sqlite3GlobalConfig.m.xRoundup(sizeof(HashElem)) * (
              len(schema.Tables)
            + len(schema.Triggers)
            + len(schema.Indices)
            + len(schema.ForeignKeys)
          )
          nByte += sqlite3MallocSize(schema.Tables.ht);
          nByte += sqlite3MallocSize(schema.Triggers.ht);
          nByte += sqlite3MallocSize(schema.Indices.ht);
          nByte += sqlite3MallocSize(schema.ForeignKeys.ht);

          for _, trigger := range schema.Triggers {
            db.DeleteTrigger(trigger)
          }
		  for _, p := range schema.Tables {
		  	db.DeleteTable(p)
          }
        }
      }
      db.pnBytesFreed = 0;
      db.LeaveBtreeAll()

      *pHighwater = 0;
      *pCurrent = nByte;
      break;
    }
*/
    /*
    ** *pCurrent gets an accurate estimate of the amount of memory used
    ** to store all prepared statements.
    ** *pHighwater is set to zero.
    */
    case SQLITE_DBSTATUS_STMT_USED: {
      struct Vdbe *pVdbe;         /* Used to iterate through VMs */
      int nByte = 0;              /* Used to accumulate return value */

      db.pnBytesFreed = &nByte;
      for(pVdbe=db.pVdbe; pVdbe; pVdbe=pVdbe.Next){
        db.DeleteObject(pVdbe)
      }
      db.pnBytesFreed = 0;

      *pHighwater = 0;
      *pCurrent = nByte;

      break;
    }

    /*
    ** Set *pCurrent to the total cache hits or misses encountered by all
    ** pagers the database handle is connected to. *pHighwater is always set 
    ** to zero.
    */
    case SQLITE_DBSTATUS_CACHE_HIT:
    case SQLITE_DBSTATUS_CACHE_MISS:
    case SQLITE_DBSTATUS_CACHE_WRITE:{
      int i;
      int nRet = 0;
      assert( SQLITE_DBSTATUS_CACHE_MISS==SQLITE_DBSTATUS_CACHE_HIT+1 );
      assert( SQLITE_DBSTATUS_CACHE_WRITE==SQLITE_DBSTATUS_CACHE_HIT+2 );

		for _, database := range db.Databases {
			if database.pBt != nil {
				pPager := database.pBt.Pager()
				sqlite3PagerCacheStat(pPager, op, resetFlag, &nRet)
			}
		}
      *pHighwater = 0;
      *pCurrent = nRet;
      break;
    }

    default: {
      rc = SQLITE_ERROR;
    }
  }
  db.mutex.Unlock()
  return rc;
}
