import "crypto/rand"

/* This file contains code associated with the ANALYZE command.
**
** The ANALYZE command gather statistics about the content of tables
** and indices.  These statistics are made available to the query planner
** to help it make better decisions about how to perform queries.
**
** The following system tables are or have been supported:
**
**    CREATE TABLE sqlite_stat1(tbl, idx, stat);
**    CREATE TABLE sqlite_stat2(tbl, idx, sampleno, sample);
**    CREATE TABLE sqlite_stat3(tbl, idx, nEq, nLt, nDLt, sample);
**
** Additional tables might be added in future releases of SQLite.
** The sqlite_stat2 table is not created or used unless the SQLite version
** is between 3.6.18 and 3.7.8, inclusive, and unless SQLite is compiled
** with SQLITE_ENABLE_STAT2.  The sqlite_stat2 table is deprecated.
** The sqlite_stat2 table is superceded by sqlite_stat3, which is only
** created and used by SQLite versions 3.7.9 and later and with
** SQLITE_ENABLE_STAT3 defined.  The fucntionality of sqlite_stat3
** is a superset of sqlite_stat2.  
**
** Format of sqlite_stat1:
**
** There is normally one row per index, with the index identified by the
** name in the idx column.  The tbl column is the name of the table to
** which the index belongs.  In each such row, the stat column will be
** a string consisting of a list of integers.  The first integer in this
** list is the number of rows in the index and in the table.  The second
** integer is the average number of rows in the index that have the same
** value in the first column of the index.  The third integer is the average
** number of rows in the index that have the same value for the first two
** columns.  The N-th integer (for N>1) is the average number of rows in 
** the index which have the same value for the first N-1 columns.  For
** a K-column index, there will be K+1 integers in the stat column.  If
** the index is unique, then the last integer will be 1.
**
** The list of integers in the stat column can optionally be followed
** by the keyword "unordered".  The "unordered" keyword, if it is present,
** must be separated from the last integer by a single space.  If the
** "unordered" keyword is present, then the query planner assumes that
** the index is unordered and will not use the index for a range query.
** 
** If the sqlite_stat1.idx column is NULL, then the sqlite_stat1.stat
** column contains a single integer which is the (estimated) number of
** rows in the table identified by sqlite_stat1.tbl.
**
** Format of sqlite_stat2:
**
** The sqlite_stat2 is only created and is only used if SQLite is compiled
** with SQLITE_ENABLE_STAT2 and if the SQLite version number is between
** 3.6.18 and 3.7.8.  The "stat2" table contains additional information
** about the distribution of keys within an index.  The index is identified by
** the "idx" column and the "tbl" column is the name of the table to which
** the index belongs.  There are usually 10 rows in the sqlite_stat2
** table for each index.
**
** The sqlite_stat2 entries for an index that have sampleno between 0 and 9
** inclusive are samples of the left-most key value in the index taken at
** evenly spaced points along the index.  Let the number of samples be S
** (10 in the standard build) and let C be the number of rows in the index.
** Then the sampled rows are given by:
**
**     rownumber = (i*C*2 + C)/(S*2)
**
** For i between 0 and S-1.  Conceptually, the index space is divided into
** S uniform buckets and the samples are the middle row from each bucket.
**
** The format for sqlite_stat2 is recorded here for legacy reference.  This
** version of SQLite does not support sqlite_stat2.  It neither reads nor
** writes the sqlite_stat2 table.  This version of SQLite only supports
** sqlite_stat3.
**
** Format for sqlite_stat3:
**
** The sqlite_stat3 is an enhancement to sqlite_stat2.  A new name is
** used to avoid compatibility problems.  
**
** The format of the sqlite_stat3 table is similar to the format of
** the sqlite_stat2 table.  There are multiple entries for each index.
** The idx column names the index and the tbl column is the table of the
** index.  If the idx and tbl columns are the same, then the sample is
** of the INTEGER PRIMARY KEY.  The sample column is a value taken from
** the left-most column of the index.  The nEq column is the approximate
** number of entires in the index whose left-most column exactly matches
** the sample.  nLt is the approximate number of entires whose left-most
** column is less than the sample.  The nDLt column is the approximate
** number of distinct left-most entries in the index that are less than
** the sample.
**
** Future versions of SQLite might change to store a string containing
** multiple integers values in the nDLt column of sqlite_stat3.  The first
** integer will be the number of prior index entires that are distinct in
** the left-most column.  The second integer will be the number of prior index
** entries that are distinct in the first two columns.  The third integer
** will be the number of prior index entries that are distinct in the first
** three columns.  And so forth.  With that extension, the nDLt field is
** similar in function to the sqlite_stat1.stat field.
**
** There can be an arbitrary number of sqlite_stat3 entries per index.
** The ANALYZE command will typically generate sqlite_stat3 tables
** that contain between 10 and 40 samples which are distributed across
** the key space, though not uniformly, and which include samples with
** largest possible nEq values.
*/

//	This routine generates code that opens the sqlite_stat1 table for writing with cursor stat1_cursor and the sqlite_stat3 table is opened for writing using cursor (stat1_cursor + 1)
//	If the sqlite_stat1 tables does not previously exist, it is created. Similarly, if the sqlite_stat3 table does not exist it is created. 
//	Argument zWhere may be a pointer to a buffer containing a table name, or it may be a NULL pointer. If it is not NULL, then all entries in the sqlite_stat1 and (if applicable) sqlite_stat3 tables associated with the named table are deleted. If zWhere==0, then code is generated to delete all stat table entries.
type stat_table struct {
	Name, Columns	string
	Root			int
	Create			byte
}
func (pParse *Parse) openStatTable(iDb, stat1_cursor int, zWhere, zWhereType string) {
	if v := pParse.GetVdbe(); v != nil {
		tables := []*stat_table{
			&stat_table{ Name: "sqlite_stat1", Columns: "tbl,idx,stat" },
			&stat_table{ Name: "sqlite_stat3", Columns: "tbl,idx,neq,nlt,ndlt,sample" },
		}

		db := pParse.db
		assert( v.DB() == db )
		pDb := &db.Databases[iDb]

		//	Create new statistic tables if they do not exist, or clear them if they do already exist.
		for i, table := range tables {
			if pStat := db.FindTable(table.Name, pDb.Name); pStat != nil {
				//	The table already exists. If zWhere is not "", delete all entries associated with the table zWhere else delete the entire contents of the table.
				table.Root = pStat.tnum
				pParse.TableLock(iDb, table.Root, table.Name, true)
				if zWhere {
					sqlite3NestedParse(pParse, "DELETE FROM %Q.%s WHERE %s=%Q", pDb.Name, table.Name, zWhereType, zWhere)
				} else {
					v.AddOp2(OP_Clear, table.Root, iDb)
				}
			} else {
				//	The sqlite_stat[12] table does not exist. Create it. Note that a side-effect of the CREATE TABLE statement is to leave the rootpage of the new table in register pParse.regRoot. This is important because the OpenWrite opcode below will be needing it.
				sqlite3NestedParse(pParse, "CREATE TABLE %Q.%s(%s)", pDb.Name, table.Name, table.Columns)
				table.Root = pParse.regRoot
				table.Create = 1
			}
  		}

		//	Open the tables for writing.
		for i, table := range tables {
			v.AddOp3(OP_OpenWrite, stat1_cursor + i, table.Root, iDb)
			sqlite3VdbeChangeP4(v, -1, (char *)(3), P4_INT32)
			v.ChangeP5(table.Create)
		}
	}
}

/*
** Recommended number of samples for sqlite_stat3
*/
#ifndef SQLITE_STAT3_SAMPLES
# define SQLITE_STAT3_SAMPLES 24
#endif

/*
** Three SQL functions - stat3_init(), stat3_push(), and stat3_pop() -
** share an instance of the following structure to hold their state
** information.
*/
typedef struct Stat3Accum Stat3Accum;
struct Stat3Accum {
  tRowcnt nRow;             /* Number of rows in the entire table */
  tRowcnt nPSample;         /* How often to do a periodic sample */
  int iMin;                 /* Index of entry with minimum nEq and hash */
  int mxSample;             /* Maximum number of samples to accumulate */
  int nSample;              /* Current number of samples */
  uint32 iPrn;                 /* Pseudo-random number used for sampling */
  struct Stat3Sample {
    int64 iRowid;                /* Rowid in main table of the key */
    tRowcnt nEq;               /* sqlite_stat3.nEq */
    tRowcnt nLt;               /* sqlite_stat3.nLt */
    tRowcnt nDLt;              /* sqlite_stat3.nDLt */
    byte isPSample;              /* True if a periodic sample */
    uint32 iHash;                 /* Tiebreaker hash */
  } *a;                     /* An array of samples */
};

/*
** Implementation of the stat3_init(C,S) SQL function.  The two parameters
** are the number of rows in the table or index (C) and the number of samples
** to accumulate (S).
**
** This routine allocates the Stat3Accum object.
**
** The return value is the Stat3Accum object (P).
*/
static void stat3Init(
  sqlite3_context *context,
  int argc,
  sqlite3_value **argv
){
  Stat3Accum *p;
  tRowcnt nRow;
  int mxSample;
  int n;

  nRow = (tRowcnt)sqlite3_value_int64(argv[0]);
  mxSample = sqlite3_value_int(argv[1]);
  n = sizeof(*p) + sizeof(p.a[0])*mxSample;
  p = sqlite3_malloc( n );
  if( p==0 ){
    sqlite3_result_error_nomem(context);
    return;
  }
  memset(p, 0, n);
  p.a = (struct Stat3Sample*)&p[1];
  p.nRow = nRow;
  p.mxSample = mxSample;
  p.nPSample = p.nRow/(mxSample/3+1) + 1;
  rand.Read(&p.iPrn)
  sqlite3_result_blob(context, p, sizeof(p), sqlite3_free);
}
static const FuncDef stat3InitFuncdef = {
  2,                /* nArg */
  SQLITE_UTF8,      /* iPrefEnc */
  0,                /* flags */
  0,                /* pUserData */
  0,                /* Next */
  stat3Init,        /* xFunc */
  0,                /* xStep */
  0,                /* xFinalize */
  "stat3_init",     /* Name */
  0,                /* pHash */
  0                 /* pDestructor */
};


/*
** Implementation of the stat3_push(nEq,nLt,nDLt,rowid,P) SQL function.  The
** arguments describe a single key instance.  This routine makes the 
** decision about whether or not to retain this key for the sqlite_stat3
** table.
**
** The return value is NULL.
*/
static void stat3Push(
  sqlite3_context *context,
  int argc,
  sqlite3_value **argv
){
  Stat3Accum *p = (Stat3Accum*)sqlite3_value_blob(argv[4]);
  tRowcnt nEq = sqlite3_value_int64(argv[0]);
  tRowcnt nLt = sqlite3_value_int64(argv[1]);
  tRowcnt nDLt = sqlite3_value_int64(argv[2]);
  int64 rowid = sqlite3_value_int64(argv[3]);
  byte isPSample = 0;
  byte doInsert = 0;
  int iMin = p.iMin;
  struct Stat3Sample *pSample;
  int i;
  uint32 h;

  if( nEq==0 ) return;
  h = p.iPrn = p.iPrn*1103515245 + 12345;
  if( (nLt/p.nPSample)!=((nEq+nLt)/p.nPSample) ){
    doInsert = isPSample = 1;
  }else if( p.nSample<p.mxSample ){
    doInsert = 1;
  }else{
    if( nEq>p.a[iMin].nEq || (nEq==p.a[iMin].nEq && h>p.a[iMin].iHash) ){
      doInsert = 1;
    }
  }
  if( !doInsert ) return;
  if( p.nSample==p.mxSample ){
    assert( p.nSample - iMin - 1 >= 0 );
    memmove(&p.a[iMin], &p.a[iMin+1], sizeof(p.a[0])*(p.nSample-iMin-1));
    pSample = &p.a[p.nSample-1];
  }else{
    pSample = &p.a[p.nSample++];
  }
  pSample.iRowid = rowid;
  pSample.nEq = nEq;
  pSample.nLt = nLt;
  pSample.nDLt = nDLt;
  pSample.iHash = h;
  pSample.isPSample = isPSample;

  /* Find the new minimum */
  if( p.nSample==p.mxSample ){
    pSample = p.a;
    i = 0;
    while( pSample.isPSample ){
      i++;
      pSample++;
      assert( i<p.nSample );
    }
    nEq = pSample.nEq;
    h = pSample.iHash;
    iMin = i;
    for(i++, pSample++; i<p.nSample; i++, pSample++){
      if( pSample.isPSample ) continue;
      if( pSample.nEq<nEq
       || (pSample.nEq==nEq && pSample.iHash<h)
      ){
        iMin = i;
        nEq = pSample.nEq;
        h = pSample.iHash;
      }
    }
    p.iMin = iMin;
  }
}
static const FuncDef stat3PushFuncdef = {
  5,                /* nArg */
  SQLITE_UTF8,      /* iPrefEnc */
  0,                /* flags */
  0,                /* pUserData */
  0,                /* Next */
  stat3Push,        /* xFunc */
  0,                /* xStep */
  0,                /* xFinalize */
  "stat3_push",     /* Name */
  0,                /* pHash */
  0                 /* pDestructor */
};

/*
** Implementation of the stat3_get(P,N,...) SQL function.  This routine is
** used to query the results.  Content is returned for the Nth sqlite_stat3
** row where N is between 0 and S-1 and S is the number of samples.  The
** value returned depends on the number of arguments.
**
**   argc==2    result:  rowid
**   argc==3    result:  nEq
**   argc==4    result:  nLt
**   argc==5    result:  nDLt
*/
static void stat3Get(
  sqlite3_context *context,
  int argc,
  sqlite3_value **argv
){
  int n = sqlite3_value_int(argv[1]);
  Stat3Accum *p = (Stat3Accum*)sqlite3_value_blob(argv[0]);

  assert( p!=0 );
  if( p.nSample<=n ) return;
  switch( argc ){
    case 2:  sqlite3_result_int64(context, p.a[n].iRowid); break;
    case 3:  sqlite3_result_int64(context, p.a[n].nEq);    break;
    case 4:  sqlite3_result_int64(context, p.a[n].nLt);    break;
    default: sqlite3_result_int64(context, p.a[n].nDLt);   break;
  }
}
static const FuncDef stat3GetFuncdef = {
  -1,               /* nArg */
  SQLITE_UTF8,      /* iPrefEnc */
  0,                /* flags */
  0,                /* pUserData */
  0,                /* Next */
  stat3Get,         /* xFunc */
  0,                /* xStep */
  0,                /* xFinalize */
  "stat3_get",     /* Name */
  0,                /* pHash */
  0                 /* pDestructor */
};


//	Generate code to do an analysis of all indices associated with a single table.
func (pParse *Parse) analyzeOneTable(table *Table, relevant_index *Index, stat1_cursor, base_register int) {
	jZeroRows := -1          /* Jump from here if number of rows is zero */
	regTabname := base_register     /* Register containing table name */
	regIdxname := base_register + 1     /* Register containing index name */
	regStat1 := base_register + 2       /* The stat column of sqlite_stat1 */
	regNumEq := base_register + 3     /* Number of instances.  Same as regStat1 */
	regNumLt := base_register + 4       /* Number of keys less than regSample */
	regNumDLt := base_register + 5      /* Number of distinct keys less than regSample */
	regSample := base_register + 6      /* The next sample value */
	regRowid := regSample;    /* Rowid of a sample */
	regAccum := base_register + 7       /* Register to hold Stat3Accum object */
	regLoop := base_register + 8        /* Loop counter */
	regCount := base_register + 9       /* Number of rows in the table or index */
	regTemp1 := base_register + 10       /* Intermediate register */
	regTemp2 := base_register + 11       /* Intermediate register */
	regCol := base_register + 12         /* Content of a column in analyzed table */
	regRec := base_register + 13         /* Register holding completed record */
	regTemp := base_register + 14        /* Temporary use register */
	regNewRowid := base_register + 15    /* Rowid for the inserted record */
	base_register += 16

	iTabCur := pParse.nTab++; /* Table cursor */

	v := pParse.GetVdbe()
	db := pParse.db
	iDb := db.SchemaToIndex(table.Schema)
	assert( iDb >= 0 )
	switch {
	case v == nil || table == nil:		fallthrough
	case table.tnum == 0:				fallthrough		//	Do not gather statistics on views or virtual tables
	case table.Name[0:7] == "sqlite_":	fallthrough		//	Do not gather statistics on system tables
	case pParse.AuthCheck(SQLITE_ANALYZE, table.Name, 0, db.Databases[iDb].Name) != SQLITE_OK:
		return
	}

	//	Establish a read-lock on the table at the shared-cache level.
	pParse.TableLock(iDb, table.tnum, table.Name, false)

	iIdxCur := pParse.nTab++					//	Cursor open on index being analyzed
	sqlite3VdbeAddOp4(v, OP_String8, 0, regTabname, 0, table.Name, 0)
	do_once := true
	for _, index := range table.Indices {
		if index == relevant_index || relevant_index == nil {
			v.NoopComment("Begin analysis of ", index.Name)
			nCol := len(index.Columns)
			jump_addresses := make([]int, nCol)
			pKey := pParse.IndexKeyinfo(index)
			if base_register + 1 + (nCol * 2) > pParse.nMem {
				pParse.nMem = base_register + 1 + (nCol * 2)
			}

			//	Open a cursor to the index to be analyzed.
			assert( iDb == db.SchemaToIndex(index.Schema) )
			sqlite3VdbeAddOp4(v, OP_OpenRead, iIdxCur, index.tnum, iDb, (char *)pKey, P4_KEYINFO_HANDOFF)
			v.Comment(index.Name)

			//	Populate the register containing the index name.
			sqlite3VdbeAddOp4(v, OP_String8, 0, regIdxname, 0, index.Name, 0)

			if do_once {
				pParse.OpenTable(table, iTabCur, iDb, OP_OpenRead)
				do_once = false
			}
			v.AddOp2(OP_Count, iIdxCur, regCount)
			v.AddOp2(OP_Integer, SQLITE_STAT3_SAMPLES, regTemp1)
			v.AddOp2(OP_Integer, 0, regNumEq)
			v.AddOp2(OP_Integer, 0, regNumLt)
			v.AddOp2(OP_Integer, -1, regNumDLt)
			v.AddOp3(OP_Null, 0, regSample, regAccum)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regCount, regAccum, (char*)&stat3InitFuncdef, P4_FUNCDEF)
			v.ChangeP5(2)

			//	The block of memory cells initialized here is used as follows.
			//		base_register:												The total number of rows in the table.
			//		base_register + 1 .. base_register + nCol:					Number of distinct entries in index considering the left-most N columns only, where N is between 1 and nCol, inclusive.
			//		base_register + nCol + 1 .. base_register + 2 * nCol:		Previous value of indexed columns, from left to right.
			//	Cells base_register through base_register + nCol are initialized to 0. The others are initialized to contain an SQL NULL.
			for i := 0; i <= nCol; i++ {
				v.AddOp2(OP_Integer, 0, base_register + i)
			}
			for i := 0; i < nCol; i++ {
				v.AddOp2(OP_Null, 0, base_register + nCol + i + 1)
			}

			//	Start the analysis loop. This loop runs through all the entries in the index b-tree.
			endOfLoop := v.MakeLabel()
			v.AddOp2(OP_Rewind, iIdxCur, endOfLoop)
			topOfLoop := v.CurrentAddr()
			v.AddOp2(OP_AddImm, base_register, 1)						//	Increment row counter

			addrIfNot := 0										//	address of OP_IfNot
			for i := 0; i < nCol; i++ {
				v.AddOp3(OP_Column, iIdxCur, i, regCol)
				if i == 0 {
					//	Always record the very first row
					addrIfNot = v.AddOp1(OP_IfNot, base_register + 1)
				}
				assert( index.azColl != nil )
				assert( index.azColl[i] != nil )
				pColl := pParse.LocateCollSeq(index.azColl[i])
				jump_addresses[i] = sqlite3VdbeAddOp4(v, OP_Ne, regCol, 0, base_register + nCol + i + 1, (char*)pColl, P4_COLLSEQ)
				v.ChangeP5(SQLITE_NULLEQ)
				v.Comment("jump if column ", i, " changed", i)
				if i == 0 {
					v.AddOp2(OP_AddImm, regNumEq, 1)
					v.Comment("incr repeat count")
				}
			}
			v.AddOp2(OP_Goto, 0, endOfLoop)
			for i := 0; i < nCol; i++ {
				v.JumpHere(jump_addresses[i])			//	Set jump dest for the OP_Ne
				if i == 0 {
					v.JumpHere(addrIfNot)				//	Jump dest for OP_IfNot
					sqlite3VdbeAddOp4(v, OP_Function, 1, regNumEq, regTemp2, (char*)&stat3PushFuncdef, P4_FUNCDEF)
					v.ChangeP5(5)
					v.AddOp3(OP_Column, iIdxCur, len(index.Columns), regRowid)
					v.AddOp3(OP_Add, regNumEq, regNumLt, regNumLt)
					v.AddOp2(OP_AddImm, regNumDLt, 1)
					v.AddOp2(OP_Integer, 1, regNumEq)
				}
				v.AddOp2(OP_AddImm, base_register + i + 1, 1)
				v.AddOp3(OP_Column, iIdxCur, i, base_register + nCol + i + 1)
			}

			//	Always jump here after updating the base_register + 1...base_register + 1 + nCol counters
			v.ResolveLabel(endOfLoop)

			v.AddOp2(OP_Next, iIdxCur, topOfLoop)
			v.AddOp1(OP_Close, iIdxCur)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regNumEq, regTemp2, (char*)&stat3PushFuncdef, P4_FUNCDEF)
			v.ChangeP5(5)
			v.AddOp2(OP_Integer, -1, regLoop)
			shortJump := v.AddOp2(OP_AddImm, regLoop, 1)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regAccum, regTemp1, (char*)&stat3GetFuncdef, P4_FUNCDEF)
			v.ChangeP5(2)
			v.AddOp1(OP_IsNull, regTemp1)
			v.AddOp3(OP_NotExists, iTabCur, shortJump, regTemp1)
			v.AddOp3(OP_Column, iTabCur, index.Columns[0], regSample)
			v.ColumnDefault(table, index.Columns[0], regSample)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regAccum, regNumEq, (char*)&stat3GetFuncdef, P4_FUNCDEF)
			v.ChangeP5(3)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regAccum, regNumLt, (char*)&stat3GetFuncdef, P4_FUNCDEF)
			v.ChangeP5(4)
			sqlite3VdbeAddOp4(v, OP_Function, 1, regAccum, regNumDLt, (char*)&stat3GetFuncdef, P4_FUNCDEF)
			v.ChangeP5(5)
			sqlite3VdbeAddOp4(v, OP_MakeRecord, regTabname, 6, regRec, "bbbbbb", 0)
			v.AddOp2(OP_NewRowid, stat1_cursor + 1, regNewRowid)
			v.AddOp3(OP_Insert, stat1_cursor + 1, regRec, regNewRowid)
			v.AddOp2(OP_Goto, 0, shortJump)
			v.JumpHere(shortJump + 2)

			//	Store the results in sqlite_stat1.
			//	The result is a single row of the sqlite_stat1 table. The first two columns are the names of the table and index. The third column is a string composed of a list of integer statistics about the index. The first integer in the list is the total number of entries in the index. There is one additional integer in the list for each column of the table. This additional integer is a guess of how many rows of the table the index will select. If D is the count of distinct values and K is the total number of rows, then the integer is computed as:
			//			I = (K + D - 1) / D
			//	If K == 0 then no entry is made into the sqlite_stat1 table.  
			//	If K > 0 then it is always the case the D>0 so division by zero is never possible.
			v.AddOp2(OP_SCopy, base_register, regStat1)
			if jZeroRows < 0 {
				jZeroRows = v.AddOp1(OP_IfNot, base_register)
			}
			for i := 0; i < nCol; i++ {
				sqlite3VdbeAddOp4(v, OP_String8, 0, regTemp, 0, " ", 0)
				v.AddOp3(OP_Concat, regTemp, regStat1, regStat1)
				v.AddOp3(OP_Add, base_register, base_register + i + 1, regTemp)
				v.AddOp2(OP_AddImm, regTemp, -1)
				v.AddOp3(OP_Divide, base_register + i + 1, regTemp, regTemp)
				v.AddOp1(OP_ToInt, regTemp)
				v.AddOp3(OP_Concat, regTemp, regStat1, regStat1)
			}
			sqlite3VdbeAddOp4(v, OP_MakeRecord, regTabname, 3, regRec, "aaa", 0)
			v.AddOp2(OP_NewRowid, stat1_cursor, regNewRowid)
			v.AddOp3(OP_Insert, stat1_cursor, regRec, regNewRowid)
			v.ChangeP5(OPFLAG_APPEND)
		}
	}

	//	If the table has no indices, create a single sqlite_stat1 entry containing NULL as the index name and the row count as the content.
	if len(table.Indices) == 0 {
		v.AddOp3(OP_OpenRead, iIdxCur, table.tnum, iDb)
		v.Comment(table.Name)
		v.AddOp2(OP_Count, iIdxCur, regStat1)
		v.AddOp1(OP_Close, iIdxCur)
		jZeroRows = v.AddOp1(OP_IfNot, regStat1)
	} else {
		v.JumpHere(jZeroRows)
		jZeroRows = v.AddOp0(OP_Goto)
	}
	v.AddOp2(OP_Null, 0, regIdxname)
	sqlite3VdbeAddOp4(v, OP_MakeRecord, regTabname, 3, regRec, "aaa", 0)
	v.AddOp2(OP_NewRowid, stat1_cursor, regNewRowid)
	v.AddOp3(OP_Insert, stat1_cursor, regRec, regNewRowid)
	v.ChangeP5(OPFLAG_APPEND)
	if pParse.nMem < regRec {
		pParse.nMem = regRec
	}
	v.JumpHere(jZeroRows)
}


//	Generate code that will cause the most recent index analysis to be loaded into internal hash tables where is can be used.
func (pParse *Parse) loadAnalysis(iDb int) {
	if v := pParse.GetVdbe(); v != nil {
		v.AddOp1(OP_LoadAnalysis, iDb)
	}
}

//	Generate code that will do an analysis of an entire database
func (pParse *Parse) analyzeDatabase(iDb int) {
	db := pParse.db
	schema := db.Databases[iDb].Schema
	pParse.BeginWriteOperation(0, iDb)
	stat1_cursor := pParse.nTab
	pParse.nTab += 3
	pParse.openStatTable(iDb, stat1_cursor, 0, 0)
	base_register := pParse.nMem + 1
	for _, pTab := range schema.Tables {
		pParse.analyzeOneTable(pTab, 0, stat1_cursor, base_register)
	}
	pParse.loadAnalysis(iDb)
}

//	Generate code that will do an analysis of a single table in a database. If relevant_index is not nil then it is a single index in table that should be analyzed.
func (pParse *Parse) analyzeTable(table *Table, relevant_index *Index) {
	assert( table != nil )
	iDb := pParse.db.SchemaToIndex(table.Schema)
	pParse.BeginWriteOperation(0, iDb)
	stat1_cursor := pParse.nTab
	pParse.nTab += 3;
	if relevant_index != nil {
		pParse.openStatTable(iDb, stat1_cursor, relevant_index.Name, "idx")
	} else {
		pParse.openStatTable(iDb, stat1_cursor, table.Name, "tbl")
	}
	pParse.analyzeOneTable(table, relevant_index, stat1_cursor, pParse.nMem + 1)
	pParse.loadAnalysis(iDb)
}

//	Generate code for the ANALYZE command. The parser calls this routine when it recognizes an ANALYZE command.
//			ANALYZE                            -- causes all indices in all attached databases to be analyzed.
//			ANALYZE  <database>                -- analyzes all indices the single database named.
//			ANALYZE  ?<database>.?<tablename>  -- analyzes all indices associated with the named table.
func (pParse *Parse) Analyze(Name1, Name2 token) {
	db := pParse.db
	if pParse.ReadSchema() == SQLITE_OK {
		assert( Name2 != "" || Name1 == "" )
		switch {
		case Name1 == "":
			//	Form 1:  Analyze everything
			for i, database := range db.Databases {
				//	Do not analyze the TEMP database
				if i != 1 {
					pParse.analyzeDatabase(i)
				}
			}
		case len(Name2) == 0:
			//	Form 2:  Analyze the database or table named
			if iDb := db.FindDb(Name1); iDb >= 0 {
				pParse.analyzeDatabase(iDb)
			} else {
				if z := Dequote(Name1); z != "" {
					if pIdx := db.FindIndex(z, 0); pIdx != nil {
						pParse.analyzeTable(pIdx.pTable, pIdx)
					} else if pTab := pParse.LocateTable(z, 0, false); pTab != 0 {
						pParse.analyzeTable(pTab, 0)
					}
				}
			}
		default:
			//	Form 3: Analyze the fully qualified table name
			if pTableName, iDb := pParse.TwoPartName(Name1, Name2); iDb >= 0 {
				zDb := db.Databases[iDb].Name
				if z := Dequote(pTableName); z != "" {
					if pIdx := db.FindIndex(z, zDb); pIdx != nil {
						pParse.analyzeTable(pIdx.pTable, pIdx)
					} else if (pTab = pParse.LocateTable(z, zDb, false)) != nil {
						pParse.analyzeTable(pTab, 0)
					}
				}
			}
		}
	}
}

/*
** Used to pass information from the analyzer reader through to the
** callback routine.
*/
typedef struct analysisInfo analysisInfo;
struct analysisInfo {
  sqlite3 *db;
  const char *zDatabase;
};

/*
** This callback is invoked once for each index when reading the
** sqlite_stat1 table.  
**
**     argv[0] = name of the table
**     argv[1] = name of the index (might be NULL)
**     argv[2] = results of analysis - on integer for each column
**
** Entries for which argv[1]==NULL simply record the number of rows in
** the table.
*/
static int analysisLoader(void *pData, int argc, char **argv, char **NotUsed){
  analysisInfo *pInfo = (analysisInfo*)pData;
  Index *pIndex;
  Table *pTable;
  int i, c, n;
  tRowcnt v;
  const char *z;

  assert( argc==3 );
  if( argv==0 || argv[0]==0 || argv[2]==0 ){
    return 0;
  }
  pTable = pInfo.db.FindTable(argv[0], pInfo.zDatabase)
  if( pTable==0 ){
    return 0;
  }
  if( argv[1] ){
    pIndex = pInfo.db.FindIndex(argv[1], pInfo.zDatabase)
  }else{
    pIndex = 0;
  }
  n = pIndex ? len(pIndex.Columns) : 0;
  z = argv[2];
  for(i=0; *z && i<=n; i++){
    v = 0;
    while( (c=z[0])>='0' && c<='9' ){
      v = v*10 + c - '0';
      z++;
    }
    if( i==0 ) pTable.nRowEst = v;
    if( pIndex==0 ) break;
    pIndex.aiRowEst[i] = v;
    if( *z==' ' ) z++;
    if( memcmp(z, "unordered", 10)==0 ){
      pIndex.bUnordered = 1;
      break;
    }
  }
  return 0;
}

//	If the Index.aSample variable is not NULL, delete the aSample[] array and its contents.
func (db *sqlite3) DeleteIndexSamples(index *Index) {
	if len(index.aSample) > 0 {
		for j := 0; j < index.nSample; j++ {
			p = index.aSample[j]
			if p.eType == SQLITE_TEXT || p.eType == SQLITE_BLOB {
				p.u.z = nil
			}
		}
		index.aSample = nil
	}
	if db != nil {
		index.nSample = 0
		index.aSample = 0
	}
}

/*
** Load content from the sqlite_stat3 table into the Index.aSample[]
** arrays of all indices.
*/
static int loadStat3(sqlite3 *db, const char *zDb){
  int rc;                       /* Result codes from subroutines */
  sqlite3_stmt *pStmt = 0;      /* An SQL statement being run */
  char *zSql;                   /* Text of the SQL statement */
  Index *pPrevIdx = 0;          /* Previous index in the loop */
  int idx = 0;                  /* slot in pIdx.aSample[] for next sample */
  int eType;                    /* Datatype of a sample */
  IndexSample *pSample;         /* A slot in pIdx.aSample[] */

  assert( !db.lookaside.bEnabled )
  if db.FindTable("sqlite_stat3", zDb) == nil {
    return SQLITE_OK;
  }

  zSql = fmt.Sprintf("SELECT idx,count(*) FROM %v.sqlite_stat3 GROUP BY idx", zDb);
  if( !zSql ){
    return SQLITE_NOMEM;
  }
  pStmt, _, rc = db.Prepare(zSql)
  zSql = nil
  if( rc ) return rc;

  while( sqlite3_step(pStmt)==SQLITE_ROW ){
    char *zIndex;   /* Index name */
    Index *pIdx;    /* Pointer to the index object */
    int nSample;    /* Number of samples */

    zIndex = (char *)sqlite3_column_text(pStmt, 0);
    if( zIndex==0 ) continue;
    nSample = sqlite3_column_int(pStmt, 1);
    pIdx = db.FindIndex(zIndex, zDb)
    if( pIdx==0 ) continue;
    assert( pIdx.nSample==0 );
    pIdx.nSample = nSample;
    pIdx.aSample = sqlite3DbMallocZero(db, nSample*sizeof(IndexSample));
    pIdx.avgEq = pIdx.aiRowEst[1];
    if( pIdx.aSample==0 ){
      db.mallocFailed = true
      sqlite3_finalize(pStmt);
      return SQLITE_NOMEM;
    }
  }
  rc = sqlite3_finalize(pStmt);
  if( rc ) return rc;

  zSql = fmt.Sprintf("SELECT idx,neq,nlt,ndlt,sample FROM %v.sqlite_stat3", zDb);
  if( !zSql ){
    return SQLITE_NOMEM;
  }
  pStmt, _, rc = db.Prepare(zSql)
  zSql = nil
  if( rc ) return rc;

  while( sqlite3_step(pStmt)==SQLITE_ROW ){
    char *zIndex;   /* Index name */
    Index *pIdx;    /* Pointer to the index object */
    int i;          /* Loop counter */
    tRowcnt sumEq;  /* Sum of the nEq values */

    zIndex = (char *)sqlite3_column_text(pStmt, 0);
    if( zIndex==0 ) continue;
    pIdx = db.FindIndex(zIndex, zDb)
    if( pIdx==0 ) continue;
    if( pIdx==pPrevIdx ){
      idx++;
    }else{
      pPrevIdx = pIdx;
      idx = 0;
    }
    assert( idx<pIdx.nSample );
    pSample = &pIdx.aSample[idx];
    pSample.nEq = (tRowcnt)sqlite3_column_int64(pStmt, 1);
    pSample.nLt = (tRowcnt)sqlite3_column_int64(pStmt, 2);
    pSample.nDLt = (tRowcnt)sqlite3_column_int64(pStmt, 3);
    if( idx==pIdx.nSample-1 ){
      if( pSample.nDLt>0 ){
        for(i=0, sumEq=0; i<=idx-1; i++) sumEq += pIdx.aSample[i].nEq;
        pIdx.avgEq = (pSample.nLt - sumEq)/pSample.nDLt;
      }
      if( pIdx.avgEq<=0 ) pIdx.avgEq = 1;
    }
    eType = sqlite3_column_type(pStmt, 4);
    pSample.eType = (byte)eType;
    switch( eType ){
      case SQLITE_INTEGER: {
        pSample.u.i = sqlite3_column_int64(pStmt, 4);
        break;
      }
      case SQLITE_FLOAT: {
        pSample.u.r = sqlite3_column_double(pStmt, 4);
        break;
      }
      case SQLITE_NULL: {
        break;
      }
      default: assert( eType==SQLITE_TEXT || eType==SQLITE_BLOB ); {
        const char *z = (const char *)(
              (eType==SQLITE_BLOB) ?
              sqlite3_column_blob(pStmt, 4):
              sqlite3_column_text(pStmt, 4)
           );
        int n = z ? sqlite3_column_bytes(pStmt, 4) : 0;
        pSample.nByte = n;
        if( n < 1){
          pSample.u.z = 0;
        }else{
          pSample.u.z = sqlite3DbMallocRaw(db, n);
          if( pSample.u.z==0 ){
            db.mallocFailed = true
            sqlite3_finalize(pStmt);
            return SQLITE_NOMEM;
          }
          memcpy(pSample.u.z, z, n);
        }
      }
    }
  }
  return sqlite3_finalize(pStmt);
}

/*
** Load the content of the sqlite_stat1 and sqlite_stat3 tables. The
** contents of sqlite_stat1 are used to populate the Index.aiRowEst[]
** arrays. The contents of sqlite_stat3 are used to populate the
** Index.aSample[] arrays.
**
** If the sqlite_stat1 table is not present in the database, SQLITE_ERROR
** is returned. In this case, even if the sqlite_stat3 table is present, no data is 
** read from it.
**
** If the sqlite_stat3 table is not present in the database, SQLITE_ERROR is
** returned. However, in this case, data is read from the sqlite_stat1
** table (if it is present) before returning.
**
** If an OOM error occurs, this function always sets db.mallocFailed.
** This means if the caller does not care about other errors, the return
** code may be ignored.
*/
int sqlite3AnalysisLoad(sqlite3 *db, int iDb){
  analysisInfo sInfo;
  char *zSql;
  int rc;

  assert( iDb >= 0 && iDb < len(db.Databases) );
  assert( db.Databases[iDb].pBt!=0 );

	//	Clear any prior statistics
	for _, index := range db.Databases[iDb].Schema.Indices {
		sqlite3DefaultRowEst(index)
		db.DeleteIndexSamples(index)
		index.aSample = index.aSample[:0]
	}

  /* Check to make sure the sqlite_stat1 table exists */
  sInfo.db = db;
  sInfo.zDatabase = db.Databases[iDb].Name;
  if db.FindTable("sqlite_stat1", sInfo.zDatabase) == nil {
    return SQLITE_ERROR;
  }

  /* Load new statistics out of the sqlite_stat1 table */
  zSql = fmt.Sprintf("SELECT tbl,idx,stat FROM %v.sqlite_stat1", sInfo.zDatabase);
  if( zSql==0 ){
    rc = SQLITE_NOMEM;
  }else{
    rc = sqlite3_exec(db, zSql, analysisLoader, &sInfo, 0);
    zSql = nil
  }


  /* Load the statistics from the sqlite_stat3 table. */
  if( rc==SQLITE_OK ){
    int lookasideEnabled = db.lookaside.bEnabled;
    db.lookaside.bEnabled = 0;
    rc = loadStat3(db, sInfo.zDatabase);
    db.lookaside.bEnabled = lookasideEnabled;
  }

  if( rc==SQLITE_NOMEM ){
    db.mallocFailed = true
  }
  return rc;
}