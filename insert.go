//	This file contains C code routines that are called by the parser to handle INSERT statements in SQLite.

//	Generate code that will open a table for reading.
func (p *Parse) OpenTable(table *Table, iCur, iDb, opcode int) {
	if !table.IsVirtual() {
		v = p.GetVdbe()
		assert( opcode == OP_OpenWrite || opcode == OP_OpenRead )
		if opcode == OP_OpenWrite {
			p.TableLock(iDb, table.tnum, table.Name, false)
		} else {
			p.TableLock(iDb, table.tnum, table.Name, true)
		}
		v.AddOp3(opcode, iCur, table.tnum, iDb)
		sqlite3VdbeChangeP4(v, -1, SQLITE_INT_TO_PTR(table.nCol), P4_INT32)
		v.Comment(table.Name)
	}
}

//	Return a pointer to the column affinity string associated with index pIdx. A column affinity string has one character for each column in the table, according to the affinity of the column:
//			Character      Column affinity
//			------------------------------
//			'a'            TEXT
//			'b'            NONE
//			'c'            NUMERIC
//			'd'            INTEGER
//			'e'            REAL
//	An extra 'd' is appended to the end of the string to cover the rowid that appears as the last column in every index.
//	Memory for the buffer containing the column index affinity string is managed along with the rest of the Index structure. It will be released when sqlite3DeleteIndex() is called.
func (v *Vdbe) IndexAffinityStr(pIdx *Index) string {
	if pIdx.zColAff == "" {
		//	The first time a column affinity string for a particular index is required, it is allocated and populated here. It is then stored as a member of the Index structure for subsequent use.
		//	The column affinity string will eventually be deleted by sqliteDeleteIndex() when the Index structure itself is cleaned up.
		for _, column := range pIdx.Columns {
			pIdx.zColAff = append(pIdx.zColAff, pIdx.pTable.Columns[column].affinity)
		}
		pIdx.zColAff = append(pIdx.zColAff, SQLITE_AFF_INTEGER)
	}
	return pIdx.zColAff
}

/*
** Set P4 of the most recently inserted opcode to a column affinity
** string for table pTab. A column affinity string has one character
** for each column indexed by the index, according to the affinity of the
** column:
**
**  Character      Column affinity
**  ------------------------------
**  'a'            TEXT
**  'b'            NONE
**  'c'            NUMERIC
**  'd'            INTEGER
**  'e'            REAL
*/
 void sqlite3TableAffinityStr(Vdbe *v, Table *pTab){
  /* The first time a column affinity string for a particular table
  ** is required, it is allocated and populated here. It is then 
  ** stored as a member of the Table structure for subsequent use.
  **
  ** The column affinity string will eventually be deleted by
  ** DeleteTable() when the Table structure itself is cleaned up.
  */
  if( !pTab.zColAff ){
    char *zColAff;
    int i;
    db := v.DB()

    zColAff = (char *)sqlite3DbMallocRaw(0, pTab.nCol+1);
    if( !zColAff ){
      db.mallocFailed = true
      return;
    }

    for(i=0; i<pTab.nCol; i++){
      zColAff[i] = pTab.Columns[i].affinity;
    }
    zColAff[pTab.nCol] = '\0';

    pTab.zColAff = zColAff;
  }

  sqlite3VdbeChangeP4(v, -1, pTab.zColAff, P4_TRANSIENT);
}

//	Return non-zero if the table pTab in database iDb or any of its indices have been opened at any point in the VDBE program beginning at location iStartAddr throught the end of the program. This is used to see if a statement of the form  "INSERT INTO <iDb, pTab> SELECT ..." can run without using temporary table for the results of the SELECT. 
func (p *Parse) readsTable(address, iDb int, table *Table) int {
	var pVTab 	*VTable

	if table.IsVirtual() {
		pVTab = sqlite3GetVTable(p.db, table)
	}

	v := p.GetVdbe()
	for iEnd := v.CurrentAddr(); address < iEnd; address++ {
		pOp := sqlite3VdbeGetOp(v, address)
		assert( pOp != nil )
		if pOp.opcode == OP_OpenRead && pOp.p3 == iDb {
			if tnum := pOp.p2; tnum == table.tnum {
				return SQLITE_ERROR
			} else {
				for _, index := range table.Indices {
					if tnum == index.tnum {
						return SQLITE_ERROR
					}
				}
			}
		}
		if pOp.opcode == OP_VOpen && pOp.p4.pVtab == pVTab {
			assert( pOp.p4.pVtab != nil )
			assert( pOp.p4type == P4_VTAB )
			return SQLITE_ERROR
		}
	}
	return SQLITE_OK
}

#ifndef SQLITE_OMIT_AUTOINCREMENT
/*
** Locate or create an AutoincInfo structure associated with table pTab
** which is in database iDb.  Return the register number for the register
** that holds the maximum rowid.
**
** There is at most one AutoincInfo structure per table even if the
** same table is autoincremented multiple times due to inserts within
** triggers.  A new AutoincInfo structure is created if this is the
** first use of table pTab.  On 2nd and subsequent uses, the original
** AutoincInfo structure is used.
**
** Three memory locations are allocated:
**
**   (1)  Register to hold the name of the pTab table.
**   (2)  Register to hold the maximum ROWID of pTab.
**   (3)  Register to hold the rowid in sqlite_sequence of pTab
**
** The 2nd register is the one that is returned.  That is all the
** insert routine needs to know about.
*/
static int autoIncBegin(
  Parse *pParse,      /* Parsing context */
  int iDb,            /* Index of the database holding pTab */
  Table *pTab         /* The table we are writing to */
){
  int memId = 0;      /* Register holding maximum rowid */
  if( pTab.tabFlags & TF_Autoincrement ){
    pToplevel := pParse.Toplevel()
    AutoincInfo *pInfo;

    pInfo = pToplevel.pAinc;
    while( pInfo && pInfo.pTab!=pTab ){ pInfo = pInfo.Next; }
    if( pInfo==0 ){
      pInfo = sqlite3DbMallocRaw(pParse.db, sizeof(*pInfo));
      if( pInfo==0 ) return 0;
      pInfo.Next = pToplevel.pAinc;
      pToplevel.pAinc = pInfo;
      pInfo.pTab = pTab;
      pInfo.iDb = iDb;
      pToplevel.nMem++;                  /* Register to hold name of table */
      pInfo.regCtr = ++pToplevel.nMem;  /* Max rowid register */
      pToplevel.nMem++;                  /* Rowid in sqlite_sequence */
    }
    memId = pInfo.regCtr;
  }
  return memId;
}

/*
** This routine generates code that will initialize all of the
** register used by the autoincrement tracker.  
*/
 void sqlite3AutoincrementBegin(Parse *pParse){
  AutoincInfo *p;            /* Information about an AUTOINCREMENT */
  sqlite3 *db = pParse.db;  /* The database connection */
  Db *pDb;                   /* Database only autoinc table */
  int memId;                 /* Register holding max rowid */
  int addr;                  /* A VDBE address */
  Vdbe *v = pParse.pVdbe;   /* VDBE under construction */

  /* This routine is never called during trigger-generation.  It is
  ** only called from the top-level */
  assert( pParse.pTriggerTab==0 );
  assert( pParse == pParse.Toplevel() );

  assert( v );   /* We failed long ago if this is not so */
  for(p = pParse.pAinc; p; p = p.Next){
    pDb = &db.Databases[p.iDb];
    memId = p.regCtr;
    pParse.OpenTable(pDb.Schema.pSeqTab, 0, p.iDb, OP_OpenRead)
    v.AddOp3(OP_Null, 0, memId, memId+1);
    addr = v.CurrentAddr()
    sqlite3VdbeAddOp4(v, OP_String8, 0, memId-1, 0, p.pTab.Name, 0);
    v.AddOp2(OP_Rewind, 0, addr+9);
    v.AddOp3(OP_Column, 0, 0, memId);
    v.AddOp3(OP_Ne, memId-1, addr+7, memId);
    v.ChangeP5(SQLITE_JUMPIFNULL)
    v.AddOp2(OP_Rowid, 0, memId+1);
    v.AddOp3(OP_Column, 0, 1, memId);
    v.AddOp2(OP_Goto, 0, addr+9);
    v.AddOp2(OP_Next, 0, addr+2);
    v.AddOp2(OP_Integer, 0, memId);
    v.AddOp0(OP_Close);
  }
}

/*
** Update the maximum rowid for an autoincrement calculation.
**
** This routine should be called when the top of the stack holds a
** new rowid that is about to be inserted.  If that new rowid is
** larger than the maximum rowid in the memId memory cell, then the
** memory cell is updated.  The stack is unchanged.
*/
static void autoIncStep(Parse *pParse, int memId, int regRowid){
  if( memId>0 ){
    pParse.pVdbe.AddOp2(OP_MemMax, memId, regRowid);
  }
}

/*
** This routine generates the code needed to write autoincrement
** maximum rowid values back into the sqlite_sequence register.
** Every statement that might do an INSERT into an autoincrement
** table (either directly or through triggers) needs to call this
** routine just before the "exit" code.
*/
 void sqlite3AutoincrementEnd(Parse *pParse){
  AutoincInfo *p;
  Vdbe *v = pParse.pVdbe;
  sqlite3 *db = pParse.db;

  assert( v );
  for(p = pParse.pAinc; p; p = p.Next){
    Db *pDb = &db.Databases[p.iDb];
    int j1, j2, j3, j4, j5;
    int iRec;
    int memId = p.regCtr;

    iRec = pParse.GetTempReg()
    pParse.OpenTable(pDb.Schema.pSeqTab, 0, p.iDb, OP_OpenWrite)
    j1 = v.AddOp1(OP_NotNull, memId+1);
    j2 = v.AddOp0(OP_Rewind);
    j3 = v.AddOp3(OP_Column, 0, 0, iRec);
    j4 = v.AddOp3(OP_Eq, memId-1, 0, iRec);
    v.AddOp2(OP_Next, 0, j3);
    v.JumpHere(j2)
    v.AddOp2(OP_NewRowid, 0, memId+1);
    j5 = v.AddOp0(OP_Goto);
    v.JumpHere(j4)
    v.AddOp2(OP_Rowid, 0, memId+1);
    v.JumpHere(j1)
    v.JumpHere(j5)
    v.AddOp3(OP_MakeRecord, memId-1, 2, iRec);
    v.AddOp3(OP_Insert, 0, iRec, memId+1);
    v.ChangeP5(OPFLAG_APPEND)
    v.AddOp0(OP_Close);
    pParse.ReleaseTempReg(iRec)
  }
}
#else
/*
** If SQLITE_OMIT_AUTOINCREMENT is defined, then the three routines
** above are all no-ops
*/
# define autoIncBegin(A,B,C) (0)
# define autoIncStep(A,B,C)
#endif /* SQLITE_OMIT_AUTOINCREMENT */


/* Forward declaration */
static int xferOptimization(
  Parse *pParse,        /* Parser context */
  Table *pDest,         /* The table we are inserting into */
  Select *pSelect,      /* A SELECT statement to use as the data source */
  int onError,          /* How to handle constraint errors */
  int iDbDest           /* The database of pDest */
);

/*
** This routine is call to handle SQL of the following forms:
**
**    insert into TABLE (IDLIST) values(EXPRLIST)
**    insert into TABLE (IDLIST) select
**
** The IDLIST following the table name is always optional.  If omitted,
** then a list of all columns for the table is substituted.  The IDLIST
** appears in the pColumn parameter.  pColumn is NULL if IDLIST is omitted.
**
** The pList parameter holds EXPRLIST in the first form of the INSERT
** statement above, and pSelect is NULL.  For the second form, pList is
** NULL and pSelect is a pointer to the select statement used to generate
** data for the insert.
**
** The code generated follows one of four templates.  For a simple
** select with data coming from a VALUES clause, the code executes
** once straight down through.  Pseudo-code follows (we call this
** the "1st template"):
**
**         open write cursor to <table> and its indices
**         puts VALUES clause expressions onto the stack
**         write the resulting record into <table>
**         cleanup
**
** The three remaining templates assume the statement is of the form
**
**   INSERT INTO <table> SELECT ...
**
** If the SELECT clause is of the restricted form "SELECT * FROM <table2>" -
** in other words if the SELECT pulls all columns from a single table
** and there is no WHERE or LIMIT or GROUP BY or ORDER BY clauses, and
** if <table2> and <table1> are distinct tables but have identical
** schemas, including all the same indices, then a special optimization
** is invoked that copies raw records from <table2> over to <table1>.
** See the xferOptimization() function for the implementation of this
** template.  This is the 2nd template.
**
**         open a write cursor to <table>
**         open read cursor on <table2>
**         transfer all records in <table2> over to <table>
**         close cursors
**         foreach index on <table>
**           open a write cursor on the <table> index
**           open a read cursor on the corresponding <table2> index
**           transfer all records from the read to the write cursors
**           close cursors
**         end foreach
**
** The 3rd template is for when the second template does not apply
** and the SELECT clause does not read from <table> at any time.
** The generated code follows this template:
**
**         EOF <- 0
**         X <- A
**         goto B
**      A: setup for the SELECT
**         loop over the rows in the SELECT
**           load values into registers R..R+n
**           yield X
**         end loop
**         cleanup after the SELECT
**         EOF <- 1
**         yield X
**         goto A
**      B: open write cursor to <table> and its indices
**      C: yield X
**         if EOF goto D
**         insert the select result into <table> from R..R+n
**         goto C
**      D: cleanup
**
** The 4th template is used if the insert statement takes its
** values from a SELECT but the data is being inserted into a table
** that is also read as part of the SELECT.  In the third form,
** we have to use a intermediate table to store the results of
** the select.  The template is like this:
**
**         EOF <- 0
**         X <- A
**         goto B
**      A: setup for the SELECT
**         loop over the tables in the SELECT
**           load value into register R..R+n
**           yield X
**         end loop
**         cleanup after the SELECT
**         EOF <- 1
**         yield X
**         halt-error
**      B: open temp table
**      L: yield X
**         if EOF goto M
**         insert row from R..R+n into temp table
**         goto L
**      M: open write cursor to <table> and its indices
**         rewind temp table
**      C: loop over rows of intermediate table
**           transfer values form intermediate table into <table>
**         end loop
**      D: cleanup
*/
 void sqlite3Insert(
  Parse *pParse,        /* Parser context */
  SrcList *pTabList,    /* Name of table into which we are inserting */
  ExprList *pList,      /* List of values to be inserted */
  Select *pSelect,      /* A SELECT statement to use as the data source */
  IdList *pColumn,      /* Column names corresponding to IDLIST. */
  int onError           /* How to handle constraint errors */
){
  sqlite3 *db;          /* The main database structure */
  Table *pTab;          /* The table to insert into.  aka TABLE */
  char *zTab;           /* Name of the table into which we are inserting */
  const char *zDb;      /* Name of the database holding this table */
  int i, j, idx;        /* Loop counters */
  Vdbe *v;              /* Generate code into this virtual machine */
  Index *pIdx;          /* For looping over indices of the table */
  int nColumn;          /* Number of columns in the data */
  int nHidden = 0;      /* Number of hidden columns if TABLE is virtual */
  int baseCur = 0;      /* VDBE Cursor number for pTab */
  int keyColumn = -1;   /* Column that is the INTEGER PRIMARY KEY */
  int endOfLoop;        /* Label for the end of the insertion loop */
  int useTempTable = 0; /* Store SELECT results in intermediate table */
  int srcTab = 0;       /* Data comes from this temporary cursor if >=0 */
  int addrInsTop = 0;   /* Jump to label "D" */
  int addrCont = 0;     /* Top of insert loop. Label "C" in templates 3 and 4 */
  int addrSelect = 0;   /* Address of coroutine that implements the SELECT */
  SelectDest dest;      /* Destination for SELECT on rhs of INSERT */
  int iDb;              /* Index of database holding TABLE */
  Db *pDb;              /* The database containing table being inserted into */
  int appendFlag = 0;   /* True if the insert is likely to be an append */

  /* Register allocations */
  int regFromSelect = 0;/* Base register for data coming from SELECT */
  int regAutoinc = 0;   /* Register holding the AUTOINCREMENT counter */
  int regRowCount = 0;  /* Memory cell used for the row counter */
  int regIns;           /* Block of regs holding rowid+data being inserted */
  int regRowid;         /* registers holding insert rowid */
  int regData;          /* register holding first column to insert */
  int regEof = 0;       /* Register recording end of SELECT data */
  int *aRegIdx = 0;     /* One register allocated to each index */

  int isView;                 /* True if attempting to insert into a view */
  Trigger *pTrigger;          /* List of triggers on pTab, if required */
  int tmask;                  /* Mask of trigger times */

  db = pParse.db;
  memset(&dest, 0, sizeof(dest));
  if( pParse.nErr || db.mallocFailed ){
    goto insert_cleanup;
  }

  /* Locate the table into which we will be inserting new information.
  */
  assert( len(pTabList) );
  zTab = pTabList[0].Name;
  if( zTab==0 ) goto insert_cleanup;
  pTab = pParse.SrcListLookup(pTabList)
  if( pTab==0 ){
    goto insert_cleanup;
  }
  iDb = db.SchemaToIndex(pTab.Schema)
  assert( iDb < len(db.Databases) );
  pDb = &db.Databases[iDb];
  zDb = pDb.Name;
  if pParse.AuthCheck(SQLITE_INSERT, pTab.Name, 0, zDb) {
    goto insert_cleanup;
  }

  /* Figure out if we have any triggers and if the table being
  ** inserted into is a view
  */
  pTrigger = sqlite3TriggersExist(pParse, pTab, TK_INSERT, 0, &tmask);
  isView = pTab.Select!=0;
  assert( (pTrigger && tmask) || (pTrigger==0 && tmask==0) );

  /* If pTab is really a view, make sure it has been initialized.
  ** ViewGetColumnNames() is a no-op if pTab is not a view (or virtual 
  ** module table).
  */
  if( sqlite3ViewGetColumnNames(pParse, pTab) ){
    goto insert_cleanup;
  }

  /* Ensure that:
  *  (a) the table is not read-only, 
  *  (b) that if it is a view then ON INSERT triggers exist
  */
  if( sqlite3IsReadOnly(pParse, pTab, tmask) ){
    goto insert_cleanup;
  }

  /* Allocate a VDBE
  */
  v = pParse.GetVdbe()
  if( v==0 ) goto insert_cleanup;
  if( pParse.nested==0 ) sqlite3VdbeCountChanges(v);
  pParse.BeginWriteOperation(pSelect || pTrigger, iDb)

  /* If the statement is of the form
  **
  **       INSERT INTO <table1> SELECT * FROM <table2>;
  **
  ** Then special optimizations can be applied that make the transfer
  ** very fast and which reduce fragmentation of indices.
  **
  ** This is the 2nd template.
  */
  if( pColumn==0 && xferOptimization(pParse, pTab, pSelect, onError, iDb) ){
    assert( !pTrigger );
    assert( pList==0 );
    goto insert_end;
  }

  /* If this is an AUTOINCREMENT table, look up the sequence number in the
  ** sqlite_sequence table and store it in memory cell regAutoinc.
  */
  regAutoinc = autoIncBegin(pParse, iDb, pTab);

  /* Figure out how many columns of data are supplied.  If the data
  ** is coming from a SELECT statement, then generate a co-routine that
  ** produces a single row of the SELECT on each invocation.  The
  ** co-routine is the common header to the 3rd and 4th templates.
  */
  if( pSelect ){
    /* Data is coming from a SELECT.  Generate code to implement that SELECT
    ** as a co-routine.  The code is common to both the 3rd and 4th
    ** templates:
    **
    **         EOF <- 0
    **         X <- A
    **         goto B
    **      A: setup for the SELECT
    **         loop over the tables in the SELECT
    **           load value into register R..R+n
    **           yield X
    **         end loop
    **         cleanup after the SELECT
    **         EOF <- 1
    **         yield X
    **         halt-error
    **
    ** On each invocation of the co-routine, it puts a single row of the
    ** SELECT result into registers dest.iMem...dest.iMem+dest.nMem-1.
    ** (These output registers are allocated by sqlite3Select().)  When
    ** the SELECT completes, it sets the EOF flag stored in regEof.
    */
    int rc, j1;

    regEof = ++pParse.nMem;
    v.AddOp2(OP_Integer, 0, regEof);      /* EOF <- 0 */
    v.Comment("SELECT eof flag")
    sqlite3SelectDestInit(&dest, SRT_Coroutine, ++pParse.nMem);
    addrSelect = v.CurrentAddr() + 2
    v.AddOp2(OP_Integer, addrSelect-1, dest.iParm);
    j1 = v.AddOp2(OP_Goto, 0, 0);
    v.Comment("Jump over SELECT coroutine")

    /* Resolve the expressions in the SELECT statement and execute it. */
    rc = sqlite3Select(pParse, pSelect, &dest);
    assert( pParse.nErr==0 || rc );
    if( rc || pParse.nErr || db.mallocFailed ){
      goto insert_cleanup;
    }
    v.AddOp2(OP_Integer, 1, regEof);         /* EOF <- 1 */
    v.AddOp1(OP_Yield, dest.iParm);   /* yield X */
    v.AddOp2(OP_Halt, SQLITE_INTERNAL, OE_Abort);
    v.Comment("End of SELECT coroutine")
    v.JumpHere(j1)                          /* label B: */

    regFromSelect = dest.iMem;
    assert( pSelect.pEList != nil )
    nColumn = pSelect.pEList.Len()
    assert( dest.nMem == nColumn );

    //	Set useTempTable to TRUE if the result of the SELECT statement should be written into a temporary table (template 4). Set to FALSE if each* row of the SELECT can be written directly into the destination table (template 3).
    //	A temp table must be used if the table being updated is also one of the tables being read by the SELECT statement. Also use a temp table in the case of row triggers.
    if pTrigger != nil || pParse.readsTable(addrSelect, iDb, pTab) {
      useTempTable = 1
    }

    if( useTempTable ){
      //	Invoke the coroutine to extract information from the SELECT and add it to a transient table srcTab. The code generated here is from the 4th template:
      //			B: open temp table
      //			L: yield X
      //			if EOF goto M
      //			insert row from R..R+n into temp table
      //			goto L
      //			M: ...
      int regRec;          /* Register to hold packed record */
      int regTempRowid;    /* Register to hold temp table ROWID */
      int addrTop;         /* Label "L" */
      int addrIf;          /* Address of jump to M */

      srcTab = pParse.nTab++;
      regRec = pParse.GetTempReg()
      regTempRowid = pParse.GetTempReg()
      v.AddOp2(OP_OpenEphemeral, srcTab, nColumn);
      addrTop = v.AddOp1(OP_Yield, dest.iParm);
      addrIf = v.AddOp1(OP_If, regEof);
      v.AddOp3(OP_MakeRecord, regFromSelect, nColumn, regRec);
      v.AddOp2(OP_NewRowid, srcTab, regTempRowid);
      v.AddOp3(OP_Insert, srcTab, regRec, regTempRowid);
      v.AddOp2(OP_Goto, 0, addrTop);
      v.JumpHere(addrIf)
      pParse.ReleaseTempReg(regRec)
      pParse.ReleaseTempReg(regTempRowid)
    }
  }else{
    /* This is the case if the data for the INSERT is coming from a VALUES
    ** clause
    */
    NameContext sNC;
    memset(&sNC, 0, sizeof(sNC));
    sNC.Parse = pParse;
    srcTab = -1;
    assert( useTempTable==0 );
	if pList != nil {
		nColumn = pList.Len()
	} else {
		nColumn = 0
	}
	for _, item := range pList.Items {
		if sqlite3ResolveExprNames(&sNC, item.Expr) {
			goto insert_cleanup
		}
	}
  }

  /* Make sure the number of columns in the source data matches the number
  ** of columns to be inserted into the table.
  */
  if( pTab.IsVirtual() ){
    for(i=0; i<pTab.nCol; i++){
		if pTab.Columns[i].IsHidden {
			nHidden++
		}
    }
  }
  if( pColumn==0 && nColumn && nColumn!=(pTab.nCol-nHidden) ){
    pParse.SetErrorMsg("table %v has %v columns but %v values were supplied", pTabList, pTab.nCol-nHidden, nColumn);
    goto insert_cleanup;
  }
  if( pColumn!=0 && nColumn!=pColumn.nId ){
    pParse.SetErrorMsg("%v values for %v columns", nColumn, pColumn.nId);
    goto insert_cleanup;
  }

  /* If the INSERT statement included an IDLIST term, then make sure
  ** all elements of the IDLIST really are columns of the table and 
  ** remember the column indices.
  **
  ** If the table has an INTEGER PRIMARY KEY column and that column
  ** is named in the IDLIST, then record in the keyColumn variable
  ** the index into IDLIST of the primary key column.  keyColumn is
  ** the index of the primary key as it appears in IDLIST, not as
  ** is appears in the original table.  (The index of the primary
  ** key in the original table is pTab.iPKey.)
  */
  if( pColumn ){
    for(i=0; i<pColumn.nId; i++){
      pColumn[i].idx = -1;
    }
    for(i=0; i<pColumn.nId; i++){
      for(j=0; j<pTab.nCol; j++){
        if CaseInsensitiveMatch(pColumn.a[i].Name, pTab.Columns[j].Name) {
          pColumn[i].idx = j;
          if( j==pTab.iPKey ){
            keyColumn = i;
          }
          break;
        }
      }
      if( j>=pTab.nCol ){
        if( sqlite3IsRowid(pColumn.a[i].Name) ){
          keyColumn = i;
        }else{
          pParse.SetErrorMsg("table %v has no column named %v", pTabList, pColumn.a[i].Name);
          pParse.checkSchema = 1;
          goto insert_cleanup;
        }
      }
    }
  }

  /* If there is no IDLIST term but the table has an integer primary
  ** key, the set the keyColumn variable to the primary key column index
  ** in the original table definition.
  */
  if( pColumn==0 && nColumn>0 ){
    keyColumn = pTab.iPKey;
  }
    
  /* Initialize the count of rows to be inserted
  */
  if( db.flags & SQLITE_CountRows ){
    regRowCount = ++pParse.nMem;
    v.AddOp2(OP_Integer, 0, regRowCount);
  }

  /* If this is not a view, open the table and and all indices */
  if( !isView ){
    int nIdx;

    baseCur = pParse.nTab;
    nIdx = pParse.OpenTableAndIndices(pTab, baseCur, OP_OpenWrite)
    aRegIdx = sqlite3DbMallocRaw(db, sizeof(int)*(nIdx+1));
    if( aRegIdx==0 ){
      goto insert_cleanup;
    }
    for(i=0; i<nIdx; i++){
      aRegIdx[i] = ++pParse.nMem;
    }
  }

  /* This is the top of the main insertion loop */
  if( useTempTable ){
    /* This block codes the top of loop only.  The complete loop is the
    ** following pseudocode (template 4):
    **
    **         rewind temp table
    **      C: loop over rows of intermediate table
    **           transfer values form intermediate table into <table>
    **         end loop
    **      D: ...
    */
    addrInsTop = v.AddOp1(OP_Rewind, srcTab);
    addrCont = v.CurrentAddr()
  }else if( pSelect ){
    /* This block codes the top of loop only.  The complete loop is the
    ** following pseudocode (template 3):
    **
    **      C: yield X
    **         if EOF goto D
    **         insert the select result into <table> from R..R+n
    **         goto C
    **      D: ...
    */
    addrCont = v.AddOp1(OP_Yield, dest.iParm);
    addrInsTop = v.AddOp1(OP_If, regEof);
  }

  /* Allocate registers for holding the rowid of the new row,
  ** the content of the new row, and the assemblied row record.
  */
  regRowid = regIns = pParse.nMem+1;
  pParse.nMem += pTab.nCol + 1;
  if( pTab.IsVirtual() ){
    regRowid++;
    pParse.nMem++;
  }
  regData = regRowid+1;

  /* Run the BEFORE and INSTEAD OF triggers, if there are any
  */
  endOfLoop = v.MakeLabel()
  if( tmask & TRIGGER_BEFORE ){
    regCols := pParse.GetTempRange(pTab.nCol + 1)

    /* build the NEW.* reference row.  Note that if there is an INTEGER
    ** PRIMARY KEY into which a NULL is being inserted, that NULL will be
    ** translated into a unique ID for the row.  But on a BEFORE trigger,
    ** we do not know what the unique ID will be (because the insert has
    ** not happened yet) so we substitute a rowid of -1
    */
    if( keyColumn<0 ){
      v.AddOp2(OP_Integer, -1, regCols);
    }else{
      int j1;
      if( useTempTable ){
        v.AddOp3(OP_Column, srcTab, keyColumn, regCols);
      }else{
        assert( pSelect==0 );  /* Otherwise useTempTable is true */
        sqlite3ExprCode(pParse, pList.a[keyColumn].Expr, regCols);
      }
      j1 = v.AddOp1(OP_NotNull, regCols);
      v.AddOp2(OP_Integer, -1, regCols);
      v.JumpHere(j1)
      v.AddOp1(OP_MustBeInt, regCols);
    }

    /* Cannot have triggers on a virtual table. If it were possible,
    ** this block would have to account for hidden column.
    */
    assert( !pTab.IsVirtual() );

    /* Create the new column data
    */
    for(i=0; i<pTab.nCol; i++){
      if( pColumn==0 ){
        j = i;
      }else{
        for(j=0; j<pColumn.nId; j++){
          if( pColumn[j].idx==i ) break;
        }
      }
      if( (!useTempTable && !pList) || (pColumn && j>=pColumn.nId) ){
        sqlite3ExprCode(pParse, pTab.Columns[i].pDflt, regCols+i+1);
      }else if( useTempTable ){
        v.AddOp3(OP_Column, srcTab, j, regCols+i+1); 
      }else{
        assert( pSelect==0 ); /* Otherwise useTempTable is true */
        sqlite3ExprCodeAndCache(pParse, pList.a[j].Expr, regCols+i+1);
      }
    }

    /* If this is an INSERT on a view with an INSTEAD OF INSERT trigger,
    ** do not attempt any conversions before assembling the record.
    ** If this is a real table, attempt conversions as required by the
    ** table column affinities.
    */
    if( !isView ){
      v.AddOp2(OP_Affinity, regCols+1, pTab.nCol);
      sqlite3TableAffinityStr(v, pTab);
    }

    /* Fire BEFORE or INSTEAD OF triggers */
    sqlite3CodeRowTrigger(pParse, pTrigger, TK_INSERT, 0, TRIGGER_BEFORE, 
        pTab, regCols-pTab.nCol-1, onError, endOfLoop);

    pParse.ReleaseTempRange(regCols, pTab.nCol + 1)
  }

  /* Push the record number for the new entry onto the stack.  The
  ** record number is a randomly generate integer created by NewRowid
  ** except when the table has an INTEGER PRIMARY KEY column, in which
  ** case the record number is the same as that column. 
  */
  if( !isView ){
    if( pTab.IsVirtual() ){
      /* The row that the VUpdate opcode will delete: none */
      v.AddOp2(OP_Null, 0, regIns);
    }
    if( keyColumn>=0 ){
      if( useTempTable ){
        v.AddOp3(OP_Column, srcTab, keyColumn, regRowid);
      }else if( pSelect ){
        v.AddOp2(OP_SCopy, regFromSelect+keyColumn, regRowid);
      }else{
        VdbeOp *pOp;
        sqlite3ExprCode(pParse, pList.a[keyColumn].Expr, regRowid);
        pOp = sqlite3VdbeGetOp(v, -1);
        if( pOp && pOp.opcode==OP_Null && !pTab.IsVirtual() ){
          appendFlag = 1;
          pOp.opcode = OP_NewRowid;
          pOp.p1 = baseCur;
          pOp.p2 = regRowid;
          pOp.p3 = regAutoinc;
        }
      }
      /* If the PRIMARY KEY expression is NULL, then use OP_NewRowid
      ** to generate a unique primary key value.
      */
      if( !appendFlag ){
        int j1;
        if( !pTab.IsVirtual() ){
          j1 = v.AddOp1(OP_NotNull, regRowid);
          v.AddOp3(OP_NewRowid, baseCur, regRowid, regAutoinc);
          v.JumpHere(j1)
        }else{
          j1 = v.CurrentAddr()
          v.AddOp2(OP_IsNull, regRowid, j1+2);
        }
        v.AddOp1(OP_MustBeInt, regRowid);
      }
    }else if( pTab.IsVirtual() ){
      v.AddOp2(OP_Null, 0, regRowid);
    }else{
      v.AddOp3(OP_NewRowid, baseCur, regRowid, regAutoinc);
      appendFlag = 1;
    }
    autoIncStep(pParse, regAutoinc, regRowid);

    /* Push onto the stack, data for all columns of the new entry, beginning
    ** with the first column.
    */
    nHidden = 0;
    for(i=0; i<pTab.nCol; i++){
      int iRegStore = regRowid+1+i;
      if( i==pTab.iPKey ){
        /* The value of the INTEGER PRIMARY KEY column is always a NULL.
        ** Whenever this column is read, the record number will be substituted
        ** in its place.  So will fill this column with a NULL to avoid
        ** taking up data space with information that will never be used. */
        v.AddOp2(OP_Null, 0, iRegStore);
        continue;
      }
      if( pColumn==0 ){
        if &pTab.Columns[i].IsHidden {
          assert( pTab.IsVirtual() );
          j = -1;
          nHidden++;
        }else{
          j = i - nHidden;
        }
      }else{
        for(j=0; j<pColumn.nId; j++){
          if( pColumn[j].idx==i ) break;
        }
      }
      if( j<0 || nColumn==0 || (pColumn && j>=pColumn.nId) ){
        sqlite3ExprCode(pParse, pTab.Columns[i].pDflt, iRegStore);
      }else if( useTempTable ){
        v.AddOp3(OP_Column, srcTab, j, iRegStore); 
      }else if( pSelect ){
        v.AddOp2(OP_SCopy, regFromSelect+j, iRegStore);
      }else{
        sqlite3ExprCode(pParse, pList.a[j].Expr, iRegStore);
      }
    }

    /* Generate code to check constraints and generate index keys and
    ** do the insertion.
    */
    if( pTab.IsVirtual() ){
      const char *pVTab = (const char *)sqlite3GetVTable(db, pTab);
      sqlite3VtabMakeWritable(pParse, pTab);
      sqlite3VdbeAddOp4(v, OP_VUpdate, 1, pTab.nCol+2, regIns, pVTab, P4_VTAB);
      v.ChangeP5(onError == OE_Default ? OE_Abort : onError)
      sqlite3MayAbort(pParse);
    }else
    {
      int isReplace;    /* Set to true if constraints may cause a replace */
      sqlite3GenerateConstraintChecks(pParse, pTab, baseCur, regIns, aRegIdx,
          keyColumn>=0, 0, onError, endOfLoop, &isReplace
      );
      pParse.FkCheck(pTab, 0, regIns)
      pParse.CompleteInsertion(pTab, baseCur, regIns, aRegIdx, 0, appendFlag, isReplace == 0)
    }
  }

  /* Update the count of rows that are inserted
  */
  if( (db.flags & SQLITE_CountRows)!=0 ){
    v.AddOp2(OP_AddImm, regRowCount, 1);
  }

  if( pTrigger ){
    /* Code AFTER triggers */
    sqlite3CodeRowTrigger(pParse, pTrigger, TK_INSERT, 0, TRIGGER_AFTER, 
        pTab, regData-2-pTab.nCol, onError, endOfLoop);
  }

  /* The bottom of the main insertion loop, if the data source
  ** is a SELECT statement.
  */
  v.ResolveLabel(endOfLoop)
  if( useTempTable ){
    v.AddOp2(OP_Next, srcTab, addrCont);
    v.JumpHere(addrInsTop)
    v.AddOp1(OP_Close, srcTab);
  }else if( pSelect ){
    v.AddOp2(OP_Goto, 0, addrCont);
    v.JumpHere(addrInsTop)
  }

  if( !pTab.IsVirtual() && !isView ){
    /* Close all tables opened */
    v.AddOp1(OP_Close, baseCur);
	for idx, pIdx := range pTab.Indices {
      v.AddOp1(OP_Close, idx + baseCur + 1)
    }
  }

insert_end:
  /* Update the sqlite_sequence table by storing the content of the
  ** maximum rowid counter values recorded while inserting into
  ** autoincrement tables.
  */
  if( pParse.nested==0 && pParse.pTriggerTab==0 ){
    sqlite3AutoincrementEnd(pParse);
  }

  /*
  ** Return the number of rows inserted. If this routine is 
  ** generating code because of a call to sqlite3NestedParse(), do not
  ** invoke the callback function.
  */
  if( (db.flags&SQLITE_CountRows) && !pParse.nested && !pParse.pTriggerTab ){
    v.AddOp2(OP_ResultRow, regRowCount, 1);
    sqlite3VdbeSetNumCols(v, 1);
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "rows inserted", SQLITE_STATIC);
  }

insert_cleanup:
  sqlite3SrcListDelete(db, pTabList);
  db.ExprListDelete(pList);
  sqlite3SelectDelete(db, pSelect);
  sqlite3IdListDelete(db, pColumn);
  aRegIdx = nil
}

/* Make sure "isView" and other macros defined above are undefined. Otherwise
** thely may interfere with compilation of other functions in this file
** (or in another file, if this file becomes part of the amalgamation).  */
#ifdef isView
 #undef isView
#endif
#ifdef pTrigger
 #undef pTrigger
#endif
#ifdef tmask
 #undef tmask
#endif


//	Generate code to do constraint checks prior to an INSERT or an UPDATE.
//	The input is a range of consecutive registers as follows:
//		1.  The rowid of the row after the update.
//		2.  The data in the first column of the entry after the update.
//		i.  Data from middle columns...
//		N.  The data in the last column of the entry after the update.
//	The regRowid parameter is the index of the register containing (1).
//	If isUpdate is true and rowidChng is non-zero, then rowidChng contains the address of a register containing the rowid before the update takes place. isUpdate is true for UPDATEs and false for INSERTs. If isUpdate is false, indicating an INSERT statement, then a non-zero rowidChng indicates that the rowid was explicitly specified as part of the INSERT statement. If rowidChng is false, it means that  the rowid is computed automatically in an insert or that the rowid value is not modified by an update.
//	The code generated by this routine store new index entries into registers identified by aRegIdx[]. No index entry is created for indices where aRegIdx[i] == 0.  The order of indices in aRegIdx[] is the same as the order of indices on the linked list of indices attached to the table.
//	This routine also generates code to check constraints. NOT NULL, CHECK, and UNIQUE constraints are all checked. If a constraint fails, then the appropriate action is performed. There are five possible actions: ROLLBACK, ABORT, FAIL, REPLACE, and IGNORE.
//			Constraint type  Action       What Happens
//			---------------  ----------   ----------------------------------------
//			any              ROLLBACK     The current transaction is rolled back and sqlite3_exec() returns immediately with a return code of SQLITE_CONSTRAINT.
//			any              ABORT        Back out changes from the current command only (do not do a complete rollback) then cause sqlite3_exec() to return immediately with SQLITE_CONSTRAINT.
//			any              FAIL         Sqlite3_exec() returns immediately with a return code of SQLITE_CONSTRAINT. The transaction is not rolled back and any prior changes are retained.
//			any              IGNORE       The record number and data is popped from the stack and there is an immediate jump to label ignoreDest.
//			NOT NULL         REPLACE      The NULL value is replace by the default value for that column. If the default value is NULL, the action is the same as ABORT.
//			UNIQUE           REPLACE      The other row that conflicts with the row being inserted is removed.
//			CHECK            REPLACE      Illegal.  The results in an exception.
//	Which action to take is determined by the overrideError parameter. Or if overrideError == OE_Default, then the pParse.onError parameter is used. Or if pParse.onError == OE_Default then the onError value for the constraint is used.
//	The calling routine must open a read/write cursor for pTab with cursor number "baseCur". All indices of pTab must also have open read/write cursors with cursor number baseCur + i for the i-th cursor. Except, if there is no possibility of a REPLACE action then cursors do not need to be open for indices where aRegIdx[i] == 0.
void sqlite3GenerateConstraintChecks(
  Parse *pParse,      /* The parser context */
  Table *pTab,        /* the table into which we are inserting */
  int baseCur,        /* Index of a read/write cursor pointing at pTab */
  int regRowid,       /* Index of the range of input registers */
  int *aRegIdx,       /* Register used by each index.  0 for unused indices */
  int rowidChng,      /* True if the rowid might collide with existing entry */
  int isUpdate,       /* True for UPDATE, False for INSERT */
  int overrideError,  /* Override onError to this if not OE_Default */
  int ignoreDest,     /* Jump to this label on an OE_Ignore resolution */
  int *pbMayReplace   /* OUT: Set to true if constraint may cause a replace */
){
  int i;              /* loop counter */
  Vdbe *v;            /* VDBE under constrution */
  int nCol;           /* Number of columns */
  int onError;        /* Conflict resolution strategy */
  int j1;             /* Addresss of jump instruction */
  int j2 = 0, j3;     /* Addresses of jump instructions */
  int regData;        /* Register containing first data column */
  int iCur;           /* Table cursor number */
  Index *pIdx;         /* Pointer to one of the indices */
  sqlite3 *db;         /* Database connection */
  int seenReplace = 0; /* True if REPLACE is used to resolve INT PK conflict */
  int regOldRowid = (rowidChng && isUpdate) ? rowidChng : regRowid;

  db = pParse.db;
  v = pParse.GetVdbe()
  assert( v!=0 );
  assert( pTab.Select==0 );  /* This table is not a VIEW */
  nCol = pTab.nCol;
  regData = regRowid + 1;

  /* Test all NOT NULL constraints.
  */
  for(i=0; i<nCol; i++){
    if( i==pTab.iPKey ){
      continue;
    }
    onError = pTab.Columns[i].notNull;
    if( onError==OE_None ) continue;
    if( overrideError!=OE_Default ){
      onError = overrideError;
    }else if( onError==OE_Default ){
      onError = OE_Abort;
    }
    if( onError==OE_Replace && pTab.Columns[i].pDflt==0 ){
      onError = OE_Abort;
    }
    assert( onError==OE_Rollback || onError==OE_Abort || onError==OE_Fail
        || onError==OE_Ignore || onError==OE_Replace );
    switch( onError ){
      case OE_Abort:
        sqlite3MayAbort(pParse);
      case OE_Rollback:
      case OE_Fail: {
        char *zMsg;
        v.AddOp3(OP_HaltIfNull,SQLITE_CONSTRAINT, onError, regData+i);
        zMsg = fmt.Sprintf("%v.%v may not be NULL", pTab.Name, pTab.Columns[i].Name);
        sqlite3VdbeChangeP4(v, -1, zMsg, P4_DYNAMIC);
        break;
      }
      case OE_Ignore: {
        v.AddOp2(OP_IsNull, regData+i, ignoreDest);
        break;
      }
      default: {
        assert( onError==OE_Replace );
        j1 = v.AddOp1(OP_NotNull, regData+i);
        sqlite3ExprCode(pParse, pTab.Columns[i].pDflt, regData+i);
        v.JumpHere(j1)
        break;
      }
    }
  }

	//	Test all CHECK constraints
	if pTab.pCheck != nil && (db.flags & SQLITE_IgnoreChecks) == 0 {
		pCheck := pTab.pCheck
		pParse.ckBase = regData
		if overrideError != OE_Default {
			onError = overrideError
		} else {
			onError = OE_Abort
		}
		for _, item := range pCheck.Items {
			allOk := v.MakeLabel()
			sqlite3ExprIfTrue(pParse, item.Expr, allOk, SQLITE_JUMPIFNULL)
			if onError == OE_Ignore {
				v.AddOp2(OP_Goto, 0, ignoreDest)
			} else {
				name = item.Name
				if onError == OE_Replace {
					onError = OE_Abort			//	IMP: R-15569-63625
				}
				if name != "" {
					name = fmt.Sprintf("constraint %v failed", name)
				}
				sqlite3HaltConstraint(pParse, onError, name, P4_DYNAMIC)
			}
			v.ResolveLabel(allOk)
		}
	}

  //	If we have an INTEGER PRIMARY KEY, make sure the primary key of the new record does not previously exist. Except, if this is an UPDATE and the primary key is not changing, that is OK.
  if( rowidChng ){
    onError = pTab.keyConf;
    if( overrideError!=OE_Default ){
      onError = overrideError;
    }else if( onError==OE_Default ){
      onError = OE_Abort;
    }
    
    if( isUpdate ){
      j2 = v.AddOp3(OP_Eq, regRowid, 0, rowidChng);
    }
    j3 = v.AddOp3(OP_NotExists, baseCur, 0, regRowid);
    switch( onError ){
      default: {
        onError = OE_Abort;
        /* Fall thru into the next case */
      }
      case OE_Rollback:
      case OE_Abort:
      case OE_Fail: {
        sqlite3HaltConstraint(
          pParse, onError, "PRIMARY KEY must be unique", P4_STATIC);
        break;
      }
      case OE_Replace: {
        //	If there are DELETE triggers on this table and the recursive-triggers flag is set, call GenerateRowDelete() to remove the conflicting row from the the table. This will fire the triggers and remove both the table and index b-tree entries.
        //	Otherwise, if there are no triggers or the recursive-triggers flag is not set, but the table has one or more indexes, call GenerateRowIndexDelete(). This removes the index b-tree entries only. The table b-tree entry will be replaced by the new entry when it is inserted.  
        //	If either GenerateRowDelete() or GenerateRowIndexDelete() is called, also invoke MultiWrite() to indicate that this VDBE may require statement rollback (if the statement is aborted after the delete takes place). Earlier versions called sqlite3MultiWrite() regardless, but being more selective here allows statements like:
        //			REPLACE INTO t(rowid) VALUES($newrowid)
        //	to run without a statement journal if there are no indexes on the table.
        Trigger *pTrigger = 0;
        if( db.flags&SQLITE_RecTriggers ){
          pTrigger = sqlite3TriggersExist(pParse, pTab, TK_DELETE, 0, 0);
        }
        if( pTrigger || sqlite3FkRequired(pParse, pTab, 0, 0) ){
          sqlite3MultiWrite(pParse);
          sqlite3GenerateRowDelete(
              pParse, pTab, baseCur, regRowid, 0, pTrigger, OE_Replace
          );
        }else if len(pTab.Indices) > 0 {
          sqlite3MultiWrite(pParse);
          pParse.GenerateRowIndexDelete(pTab, baseCur, 0)
        }
        seenReplace = 1;
        break;
      }
      case OE_Ignore: {
        assert( seenReplace==0 );
        v.AddOp2(OP_Goto, 0, ignoreDest);
        break;
      }
    }
    v.JumpHere(j3)
    if( isUpdate ){
      v.JumpHere(j2)
    }
  }

	  //	Test all UNIQUE constraints by creating entries for each UNIQUE index and making sure that duplicate entries do not already exist. Add the new records to the indices as we go.
  for iCur, pIdx := range pTab.Indices {
    int regIdx;
    int regR;

    if( aRegIdx[iCur]==0 ) continue;  /* Skip unused indices */

    /* Create a key for accessing the index entry */
    regIdx = pParse.GetTempRange(len(pIdx.Columns) + 1)
	for i, column := range pIdx.Columns {
		if column == pTab.iPKey {
			v.AddOp2(OP_SCopy, regRowid, regIdx + i)
		} else {
			v.AddOp2(OP_SCopy, regData + idx, regIdx + i)
		}
	}
    v.AddOp2(OP_SCopy, regRowid, regIdx+i);
    v.AddOp3(OP_MakeRecord, regIdx, len(pIdx.Columns) + 1, aRegIdx[iCur])
    sqlite3VdbeChangeP4(v, -1, v.IndexAffinityStr(pIdx), P4_TRANSIENT)
    sqlite3ExprCacheAffinityChange(pParse, regIdx, len(pIdx.Columns) + 1)

    /* Find out what action to take in case there is an indexing conflict */
    onError = pIdx.onError;
    if( onError==OE_None ){ 
      pParse.ReleaseTempRange(regIdx, len(pIdx.Columns) + 1)
      continue;  /* pIdx is not a UNIQUE index */
    }
    if( overrideError!=OE_Default ){
      onError = overrideError;
    }else if( onError==OE_Default ){
      onError = OE_Abort;
    }
    if( seenReplace ){
      if( onError==OE_Ignore ) onError = OE_Replace;
      else if( onError==OE_Fail ) onError = OE_Abort;
    }
    
    /* Check to see if the new index entry will be unique */
    regR = pParse.GetTempReg()
    v.AddOp2(OP_SCopy, regOldRowid, regR);
    j3 = sqlite3VdbeAddOp4(v, OP_IsUnique, baseCur + iCur + 1, 0, regR, SQLITE_INT_TO_PTR(regIdx), P4_INT32)
    pParse.ReleaseTempRange(regIdx, len(pIdx.Columns) + 1)

    /* Generate code that executes if the new index entry is not unique */
    assert( onError == OE_Rollback || onError == OE_Abort || onError == OE_Fail || onError == OE_Ignore || onError == OE_Replace )
    switch onError {
      case OE_Rollback:
      case OE_Abort:
      case OE_Fail: {
        int j;
        const char *zSep;

        errMsg = ""
		if len(pIdx.Columns) > 1 {
        	zSep = "columns "
		} else {
			zSep = "column "
		}
		for _, column := range pIdx.Columns {
			zCol = pTab.Columns[column].Name
			errMsg = append(errMsg, zSep, zCol)
			zSep = ", "
        }
		if len(pIdx.Columns) > 1 {
        	errMsg = append(errMsg, " are not unique")
		} else {
			errMsg = append(errMsg, " is not unique")
		}
        sqlite3HaltConstraint(pParse, onError, errMsg, 0);
        break;
      }
      case OE_Ignore: {
        assert( seenReplace==0 );
        v.AddOp2(OP_Goto, 0, ignoreDest);
        break;
      }
      default: {
        Trigger *pTrigger = 0;
        assert( onError==OE_Replace );
        sqlite3MultiWrite(pParse);
        if( db.flags&SQLITE_RecTriggers ){
          pTrigger = sqlite3TriggersExist(pParse, pTab, TK_DELETE, 0, 0);
        }
        sqlite3GenerateRowDelete(pParse, pTab, baseCur, regR, 0, pTrigger, OE_Replace)
        seenReplace = 1;
        break;
      }
    }
    v.JumpHere(j3)
    pParse.ReleaseTempReg(regR)
  }
  
  if( pbMayReplace ){
    *pbMayReplace = seenReplace;
  }
}

//	This routine generates code to finish the INSERT or UPDATE operation that was started by a prior call to sqlite3GenerateConstraintChecks. A consecutive range of registers starting at regRowid contains the rowid and the content to be inserted.
//	The arguments to this routine should be the same as the first six arguments to sqlite3GenerateConstraintChecks.
func (pParse *Parse) CompleteInsertion(table *Table, baseCur, regRowid int, registers []int, isUpdate, appendBias, useSeekResult bool)
	int regRec;

	v := pParse.GetVdbe()
	assert( v != nil )
	assert( table.Select == nil )										//	This table is not a VIEW
	for i := len(table.Indices) - 1; i > -1; i-- {
		if registers[i] != 0 {
			v.AddOp2(OP_IdxInsert, baseCur + i + 1, registers[i])
			if useSeekResult {
				v.ChangeP5(OPFLAG_USESEEKRESULT)
			}
		}
	}
	regData := regRowid + 1
	regRec := pParse.GetTempReg()
	v.AddOp3(OP_MakeRecord, regData, table.nCol, regRec)
	sqlite3TableAffinityStr(v, table)
	sqlite3ExprCacheAffinityChange(pParse, regData, table.nCol)

	pik_flags		byte
	if !pParse.nested {
		pik_flags = OPFLAG_NCHANGE
		if isUpdate {
			pik_flags |= OPFLAG_ISUPDATE
		} else {
			pik_flags |= OPFLAG_LASTROWID
		}
	}
	if appendBias {
		pik_flags |= OPFLAG_APPEND
	}
	if useSeekResult {
		pik_flags |= OPFLAG_USESEEKRESULT
	}
	v.AddOp3(OP_Insert, baseCur, regRec, regRowid)
	if !pParse.nested {
		sqlite3VdbeChangeP4(v, -1, table.Name, P4_TRANSIENT)
	}
	v.ChangeP5(pik_flags)
}

//	Generate code that will open cursors for a table and for all indices of that table. The "baseCur" parameter is the cursor number used for the table. Indices are opened on subsequent cursors.
//	Return the number of indices on the table.
func (pParse *Parse) OpenTableAndIndices(table *Table, baseCur, op int) (count int) {
	if !table.IsVirtual() {
	    iDb := pParse.db.SchemaToIndex(table.Schema)
	    v := pParse.GetVdbe()
	    assert( v != nil )
		pParse.OpenTable(table, baseCur, iDb, op)
		i := 1
		for _, index := range table.Indices {
			pKey := pParse.IndexKeyinfo(index)
			assert( index.Schema == table.Schema )
			sqlite3VdbeAddOp4(v, op, i + baseCur, pIdx.tnum, iDb, (char*)pKey, P4_KEYINFO_HANDOFF)
			v.Comment(pIdx.Name)
			i++
	    }
	    if pParse.nTab < baseCur + i {
	      pParse.nTab = baseCur + i
	    }
	    count = i - 1
	}
	return
}

//	Check to see if index pSrc is compatible as a source of data for index pDest in an insert transfer optimization. The rules for a compatible index:
//		*   The index is over the same set of columns
//		*   The same DESC and ASC markings occurs on all columns
//		*   The same onError processing (OE_Abort, OE_Ignore, etc)
//	*   The same collating sequence on each column
func (pSrc *Index) xferCompatible(pDest *Index) bool {
	assert( pDest && pSrc )
	assert( pDest.pTable != pSrc.pTable )

	if len(pDest.Columns) != len(pSrc.Columns) || pDest.onError != pSrc.onError {
		return false
	}
	for i, column := range pSrc.Columns {
		if column != pDest.Columns[i] || pSrc.aSortOrder[i] != pDest.aSortOrder[i] || !CaseInsensitiveMatch(pSrc.azColl[i],pDest.azColl[i]) {
			return false
		}
	}
	return true
}

//	Attempt the transfer optimization on INSERTs of the form
//			INSERT INTO tab1 SELECT * FROM tab2;
//	The xfer optimization transfers raw records from tab2 over to tab1. Columns are not decoded and reassemblied, which greatly improves performance. Raw index records are transferred in the same way.
//	The xfer optimization is only attempted if tab1 and tab2 are compatible. There are lots of rules for determining compatibility - see comments embedded in the code for details.
//	This routine returns TRUE if the optimization is guaranteed to be used. Sometimes the xfer optimization will only work if the destination table is empty - a factor that can only be determined at run-time. In that case, this routine generates code for the xfer optimization but also does a test to see if the destination table is empty and jumps over the xfer optimization code if the test fails. In that case, this routine returns FALSE so that the caller will know to go ahead and generate an unoptimized transfer. This routine also returns FALSE if there is no chance that the xfer optimization can be applied.
//	This optimization is particularly useful at making VACUUM run faster.
func (p *Parse) xferOptimization(pDest *Table, pSelect *Select, onError, iDbDest int) (ok bool) {
static int xferOptimization(
  Parse *pParse,        /* Parser context */
  Table *pDest,         /* The table we are inserting into */
  Select *pSelect,      /* A SELECT statement to use as the data source */
  int onError,          /* How to handle constraint errors */
  int iDbDest           /* The database of pDest */
){
	ExprList *pEList;                /* The result set of the SELECT */
	Index *pSrcIdx, *pDestIdx;       /* Source and destination indices */
	struct SrcList_item *pItem;      /* An element of pSelect.pSrc */
	int i;                           /* Loop counter */

	int iSrc, iDest;                 /* Cursors from source and destination */
	int addr1, addr2;                /* Loop addresses */
	int emptyDestTest;               /* Address of test for empty pDest */
	int emptySrcTest;                /* Address of test for empty pSrc */

	KeyInfo *pKey;                   /* Key information for an index */
	int regAutoinc;                  /* Memory register used by AUTOINC */
	int destHasUniqueIdx = 0;        /* True if pDest has a UNIQUE index */
	int regData, regRowid;           /* Registers holding data and rowid */


	switch {
	case pSelect == nil:						fallthrough			//	Must be of the form  INSERT INTO ... SELECT ...
	case pParse.TriggerList(pDest):				fallthrough			//	tab1 must not have triggers
	case pDest.tabFlags & TF_Virtual != 0:		fallthrough			//	tab1 must not be a virtual table
	case len(pSelect.pSrc) != 1:				fallthrough			//	FROM clause must have exactly one term
	case pSelect.pSrc[0].Select != nil:			fallthrough			//	FROM clause cannot contain a subquery
	case pSelect.Where != nil:					fallthrough			//	SELECT may not have a WHERE clause
	case pSelect.pOrderBy != nil:				fallthrough			//	SELECT may not have an ORDER BY clause
	//	Do not need to test for a HAVING clause. If HAVING is present but there is no ORDER BY, we will get an error.
	case pSelect.pGroupBy != nil:				fallthrough			//	SELECT may not have a GROUP BY clause
	case pSelect.pLimit != nil:					fallthrough			//	SELECT may not have a LIMIT clause
	case pSelect.pPrior != nil:					fallthrough			//	SELECT may not be a compound query
	case pSelect.selFlags & SF_Distinct != 0:	fallthrough			//	SELECT may not be DISTINCT
	case pSelect.pEList == nil:					fallthrough			//	SELECT must have a result set
	case pSelect.pEList.Len() != 1:				fallthrough			//	The result set must have exactly one column
	case pEList.a[0].Expr.op != TK_ALL:							//	The result set must be the special operator "*"
		return
	}
	if onError == OE_Default {
		if pDest.iPKey >= 0 {
			onError = pDest.keyConf
		}
		if onError == OE_Default {
			onError = OE_Abort
		}
	}
	pEList := pSelect.pEList

	//	At this point we have established that the statement is of the correct syntactic form to participate in this optimization. Now we have to check the semantics.
	pItem := pSelect.pSrc.a
	pSrc := pParse.LocateTable(pItem.Name, pItem.zDatabase, false)
	switch {
	case pSrc == nil:																fallthrough			//	FROM clause does not contain a real table
	case pSrc == pDest:																fallthrough			//	tab1 and tab2 may not be the same table
	case pSrc.tabFlags & TF_Virtual != 0:											fallthrough			//	tab2 must not be a virtual table
	case pSrc.Select:																fallthrough			//	tab2 may not be a view
	case pDest.nCol != pSrc.nCol:													fallthrough			//	Number of columns must be the same in tab1 and tab2
	case pDest.iPKey!=pSrc.iPKey:													fallthrough			//	Both tables must have the same INTEGER PRIMARY KEY */
	case pDest.pCheck != nil && sqlite3ExprListCompare(pSrc.pCheck, pDest.pCheck):						//	Tables have different CHECK constraints.  Ticket #2252
		return
	}
	for i := 0; i < pDest.nCol; i++ {
		switch {
		case pDest.Columns[i].affinity != pSrc.Columns[i].affinity:					fallthrough		//	Affinity must be the same on all columns
		case !CaseInsensitiveMatch(pDest.Columns[i].zColl, pSrc.Columns[i].zColl):	fallthrough		//	Collating sequence must be the same on all columns
		case pDest.Columns[i].notNull && !pSrc.Columns[i].notNull:									//	tab2 must be NOT NULL if tab1 is
			return
		}
	}
	for _, pDestIdx := range pDest.Indices {
		if pDestIdx.onError != OE_None {
			destHasUniqueIdx = 1
		}
		for _, pSrcIdx = range pSrc.Indices {
			if pSrcIdx.xferCompatible(pDestIdx) {
				break
			}
		}
		if pSrcIdx == nil {		//	pDestIdx has no corresponding index in pSrc
			return
		}
	}

	//	Disallow the transfer optimization if the destination table constains any foreign key constraints. This is more restrictive than necessary. But the main beneficiary of the transfer optimization is the VACUUM command, and the VACUUM command disables foreign key constraints. So the extra complication to make this rule less restrictive is probably not worth the effort. Ticket [6284df89debdfa61db8073e062908af0c9b6118e]
	if pParse.db.flags & SQLITE_ForeignKeys != 0 && pDest.ForeignKey != nil {
		return
	}
	if pParse.db.flags & SQLITE_CountRows != 0 {
		return					//	xfer opt does not play well with PRAGMA count_changes
	}

	//	If we get this far, it means that the xfer optimization is at least a possibility, though it might only work if the destination table (tab1) is initially empty.
	iDbSrc := pParse.db.SchemaToIndex(pSrc.Schema)
	v := pParse.GetVdbe()
	pParse.CodeVerifySchema(iDbSrc)
	iSrc = pParse.nTab++
	iDest = pParse.nTab++
	regAutoinc = autoIncBegin(pParse, iDbDest, pDest)
	pParse.OpenTable(pDest, iDest, iDbDest, OP_OpenWrite)
	if (pDest.iPKey < 0 && len(pDest.Indices) > 0) || destHasUniqueIdx || (onError != OE_Abort && onError != OE_Rollback) {
		//	In some circumstances, we are able to run the xfer optimization only if the destination table is initially empty. This code makes that determination. Conditions under which the destination must be empty:
		//		(1) There is no INTEGER PRIMARY KEY but there are indices. (If the destination is not initially empty, the rowid fields of index entries might need to change.)
		//		(2) The destination has a unique index.  (The xfer optimization is unable to test uniqueness.)
		//		(3) onError is something other than OE_Abort and OE_Rollback.
		addr1 = v.AddOp2(OP_Rewind, iDest, 0)
		emptyDestTest = v.AddOp2(OP_Goto, 0, 0)
		v.JumpHere(addr1)
	} else {
		emptyDestTest = 0
	}
	pParse.OpenTable(pSrc, iSrc, iDbSrc, OP_OpenRead)
	emptySrcTest = v.AddOp2(OP_Rewind, iSrc, 0)
	regData = pParse.GetTempReg()
	regRowid = pParse.GetTempReg()
	switch {
	case pDest.iPKey >= 0:
		addr1 = v.AddOp2(OP_Rowid, iSrc, regRowid)
		addr2 = v.AddOp3(OP_NotExists, iDest, 0, regRowid)
		sqlite3HaltConstraint(pParse, onError, "PRIMARY KEY must be unique", P4_STATIC)
		v.JumpHere(addr2)
		autoIncStep(pParse, regAutoinc, regRowid)
	case pDest.Indices == nil:
		addr1 = v.AddOp2(OP_NewRowid, iDest, regRowid)
	default:
		addr1 = v.AddOp2(OP_Rowid, iSrc, regRowid)
		assert( (pDest.tabFlags & TF_Autoincrement) == 0 )
	}
	v.AddOp2(OP_RowData, iSrc, regData)
	v.AddOp3(OP_Insert, iDest, regData, regRowid)
	v.ChangeP5(OPFLAG_NCHANGE | OPFLAG_LASTROWID | OPFLAG_APPEND)
	sqlite3VdbeChangeP4(v, -1, pDest.Name, 0)
	v.AddOp2(OP_Next, iSrc, addr1)
	for _, pDestIdx := range pDest.Indices {
		for _, pSrcIdx := range pSrc.Indices {
			if pSrcIdx.xferCompatible(pDestIdx) {
				v.AddOp2(OP_Close, iSrc, 0)
				v.AddOp2(OP_Close, iDest, 0)
				pKey = pParse.IndexKeyinfo(pSrcIdx)
				sqlite3VdbeAddOp4(v, OP_OpenRead, iSrc, pSrcIdx.tnum, iDbSrc, (char*)pKey, P4_KEYINFO_HANDOFF)
				v.Comment(pSrcIdx.Name)
				pKey = pParse.IndexKeyinfo(pDestIdx)
				sqlite3VdbeAddOp4(v, OP_OpenWrite, iDest, pDestIdx.tnum, iDbDest, (char*)pKey, P4_KEYINFO_HANDOFF)
				v.Comment(pDestIdx.Name)
				addr1 = v.AddOp2(OP_Rewind, iSrc, 0)
				v.AddOp2(OP_RowKey, iSrc, regData)
				v.AddOp3(OP_IdxInsert, iDest, regData, 1)
				v.AddOp2(OP_Next, iSrc, addr1 + 1)
				v.JumpHere(addr1)
				break
			}
		}
	}
	v.JumpHere(emptySrcTest)
	pParse.ReleaseTempReg(regRowid)
	pParse.ReleaseTempReg(regData)
	v.AddOp2(OP_Close, iSrc, 0)
	v.AddOp2(OP_Close, iDest, 0)
	if emptyDestTest {
		v.AddOp2(OP_Halt, SQLITE_OK, 0)
		v.JumpHere(emptyDestTest)
		v.AddOp2(OP_Close, iDest, 0)
		return false
	}
	return true
}