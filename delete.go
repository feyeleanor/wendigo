//	This file contains routines that are called by the parser in order to generate code for DELETE FROM statements.

//	While a SrcList can in general represent multiple tables and subqueries (as in the FROM clause of a SELECT statement) in this case it contains the name of a single table, as one might find in an INSERT, DELETE, or UPDATE statement. Look up that table in the symbol table and return a pointer. Set an error message and return NULL if the table name is not found or if any other error occurs.
//	The following fields are initialized appropriate in pSrc:
//			pSrc.a[0].pTab       Pointer to the Table object
//			pSrc.a[0].pIndex     Pointer to the INDEXED BY index, if there is one
func (pParse *Parse) SrcListLookup(pSrc *SrcList) (table *Table) {
	pItem := pSrc[0]
	assert( pItem && len(pSrc) == 1 )
	table = pParse.LocateTable(pItem.Name, pItem.zDatabase, false)
	pParse.db.DeleteTable(pItem.pTab)
	pItem.pTab = table
	if table != nil {
		table.nRef++
	}
	if pParse.IndexedByLookup(pItem) != SQLITE_OK {
		table = nil
	}
	return
}

//	Check to make sure the given table is writable. If it is not writable, generate an error message and return 1. If it is writable return 0;
int sqlite3IsReadOnly(Parse *pParse, Table *pTab, int viewOk){
  /* A table is not writable under the following circumstances:
  **
  **   1) It is a virtual table and no implementation of the xUpdate method
  **      has been provided, or
  **   2) It is a system table (i.e. sqlite_master), this call is not
  **      part of a nested parse and writable_schema pragma has not 
  **      been specified.
  **
  ** In either case leave an error message in pParse and return non-zero.
  */
  if( ( pTab.IsVirtual() 
     && sqlite3GetVTable(pParse.db, pTab).Module.Callbacks.xUpdate==0 )
   || ( (pTab.tabFlags & TF_Readonly)!=0
     && (pParse.db.flags & SQLITE_WriteSchema)==0
     && pParse.nested==0 )
  ){
    pParse.SetErrorMsg("table %v may not be modified", pTab.Name);
    return 1;
  }

  if( !viewOk && pTab.Select ){
    pParse.SetErrorMsg("cannot modify %v because it is a view",pTab.Name);
    return 1;
  }
  return 0;
}


/*
** Evaluate a view and store its result in an ephemeral table.  The
** pWhere argument is an optional WHERE clause that restricts the
** set of rows in the view that are to be added to the ephemeral table.
*/
 void sqlite3MaterializeView(
  Parse *pParse,       /* Parsing context */
  Table *pView,        /* View definition */
  Expr *pWhere,        /* Optional WHERE clause to be added */
  int iCur             /* Cursor number for ephemerial table */
){
  SelectDest dest;
  Select *pDup;
  sqlite3 *db = pParse.db;

  pDup = pView.Select.Dup();
  if( pWhere ){
    SrcList *pFrom;
    
    pWhere = pWhere.Dup();
    pFrom = db.SrcListAppend(nil, "", "");
    if( pFrom ){
      assert( len(pFrom) == 1 )
      pFrom[0].zAlias = sqlite3DbStrDup(db, pView.Name);
      pFrom[0].Select = pDup;
      assert( pFrom[0].pOn==0 );
      assert( pFrom[0].pUsing==0 );
    }else{
      sqlite3SelectDelete(db, pDup);
    }
    pDup = sqlite3SelectNew(pParse, 0, pFrom, pWhere, 0, 0, 0, 0, 0, 0);
  }
  sqlite3SelectDestInit(&dest, SRT_EphemTab, iCur);
  sqlite3Select(pParse, pDup, &dest);
  sqlite3SelectDelete(db, pDup);
}

#if defined(SQLITE_ENABLE_UPDATE_DELETE_LIMIT)
/*
** Generate an expression tree to implement the WHERE, ORDER BY,
** and LIMIT/OFFSET portion of DELETE and UPDATE statements.
**
**     DELETE FROM table_wxyz WHERE a<5 ORDER BY a LIMIT 1;
**                            \__________________________/
**                               pLimitWhere (pInClause)
*/
 Expr *sqlite3LimitWhere(
  Parse *pParse,               /* The parser context */
  SrcList *pSrc,               /* the FROM clause -- which tables to scan */
  Expr *pWhere,                /* The WHERE clause.  May be null */
  ExprList *pOrderBy,          /* The ORDER BY clause.  May be null */
  Expr *pLimit,                /* The LIMIT clause.  May be null */
  Expr *pOffset,               /* The OFFSET clause.  May be null */
  char *zStmtType              /* Either DELETE or UPDATE.  For error messages. */
){
  Expr *pWhereRowid = NULL;    /* WHERE rowid .. */
  Expr *pInClause = NULL;      /* WHERE rowid IN ( select ) */
  Expr *pSelectRowid = NULL;   /* SELECT rowid ... */
  ExprList *pEList = NULL;     /* Expression list contaning only pSelectRowid */
  SrcList *pSelectSrc = NULL;  /* SELECT rowid FROM x ... (dup of pSrc) */
  Select *pSelect = NULL;      /* Complete SELECT tree */

  /* Check that there isn't an ORDER BY without a LIMIT clause.
  */
  if( pOrderBy && (pLimit == 0) ) {
    pParse.SetErrorMsg("ORDER BY without LIMIT on %v", zStmtType);
    goto limit_where_cleanup_2;
  }

  /* We only need to generate a select expression if there
  ** is a limit/offset term to enforce.
  */
  if( pLimit == 0 ) {
    /* if pLimit is null, pOffset will always be null as well. */
    assert( pOffset == 0 );
    return pWhere;
  }

  /* Generate a select expression tree to enforce the limit/offset 
  ** term for the DELETE or UPDATE statement.  For example:
  **   DELETE FROM table_a WHERE col1=1 ORDER BY col2 LIMIT 1 OFFSET 1
  ** becomes:
  **   DELETE FROM table_a WHERE rowid IN ( 
  **     SELECT rowid FROM table_a WHERE col1=1 ORDER BY col2 LIMIT 1 OFFSET 1
  **   );
  */

  pSelectRowid = pParse.Expr(TK_ROW, nil, nil, "")
  if( pSelectRowid == 0 ) goto limit_where_cleanup_2;
  pEList = NewExprList(pSelectRowid)
  if( pEList == 0 ) goto limit_where_cleanup_2;

  //	duplicate the FROM clause as it is needed by both the DELETE/UPDATE tree and the SELECT subtree.
  pSelectSrc = pSrc.Dup(0)
  if( pSelectSrc == 0 ) {
    pParse.db.ExprListDelete(pEList);
    goto limit_where_cleanup_2;
  }

  /* generate the SELECT expression tree. */
  pSelect = sqlite3SelectNew(pParse,pEList,pSelectSrc,pWhere,0,0,
                             pOrderBy,0,pLimit,pOffset);
  if( pSelect == 0 ) return 0;

  /* now generate the new WHERE rowid IN clause for the DELETE/UDPATE */
  pWhereRowid = pParse.Expr(TK_ROW, nil, nil, "")
  if( pWhereRowid == 0 ) goto limit_where_cleanup_1;
  pInClause = pParse.Expr(TK_IN, pWhereRowid, nil, "")
  if( pInClause == 0 ) goto limit_where_cleanup_1;

  pInClause.x.Select = pSelect;
  pInClause.flags |= EP_xIsSelect;
  return pInClause;

  /* something went wrong. clean up anything allocated. */
limit_where_cleanup_1:
  sqlite3SelectDelete(pParse.db, pSelect);
  return 0;

limit_where_cleanup_2:
  pParse.db.ExprDelete(pWhere)
  pParse.db.ExprListDelete(pOrderBy);
  pParse.db.ExprDelete(pLimit)
  pParse.db.ExprDelete(pOffset)
  return 0;
}
#endif /* defined(SQLITE_ENABLE_UPDATE_DELETE_LIMIT) */

/*
** Generate code for a DELETE FROM statement.
**
**     DELETE FROM table_wxyz WHERE a<5 AND b NOT NULL;
**                 \________/       \________________/
**                  pTabList              pWhere
*/
 void sqlite3DeleteFrom(
  Parse *pParse,         /* The parser context */
  SrcList *pTabList,     /* The table from which we should delete things */
  Expr *pWhere           /* The WHERE clause.  May be null */
){
  Vdbe *v;               /* The virtual database engine */
  Table *pTab;           /* The table from which records will be deleted */
  const char *zDb;       /* Name of database holding pTab */
  int end, addr = 0;     /* A couple addresses of generated code */
  int i;                 /* Loop counter */
  WhereInfo *pWInfo;     /* Information about the WHERE clause */
  Index *pIdx;           /* For looping over indices of the table */
  int iCur;              /* VDBE Cursor number for pTab */
  sqlite3 *db;           /* Main database structure */
  AuthContext sContext;  /* Authorization context */
  NameContext sNC;       /* Name context to resolve expressions in */
  int iDb;               /* Database number */
  int memCnt = -1;       /* Memory cell used for change counting */
  int rcauth;            /* Value returned by authorization callback */

  int isView;                  /* True if attempting to delete from a view */
  Trigger *pTrigger;           /* List of table triggers, if required */

  memset(&sContext, 0, sizeof(sContext));
  db = pParse.db;
  if( pParse.nErr || db.mallocFailed ){
    goto delete_from_cleanup;
  }
  assert( pTabList.nSrc==1 );

  /* Locate the table which we want to delete.  This table has to be
  ** put in an SrcList structure because some of the subroutines we
  ** will be calling are designed to work with multiple tables and expect
  ** an SrcList* parameter instead of just a Table* parameter.
  */
  pTab = pParse.SrcListLookup(pTabList)
  if( pTab==0 )  goto delete_from_cleanup;

  /* Figure out if we have any triggers and if the table being
  ** deleted from is a view
  */
  pTrigger = sqlite3TriggersExist(pParse, pTab, TK_DELETE, 0, 0);
  isView = pTab.Select!=0;

  /* If pTab is really a view, make sure it has been initialized.
  */
  if( sqlite3ViewGetColumnNames(pParse, pTab) ){
    goto delete_from_cleanup;
  }

  if( sqlite3IsReadOnly(pParse, pTab, (pTrigger?1:0)) ){
    goto delete_from_cleanup;
  }
  iDb = db.SchemaToIndex(pTab.Schema)
  assert( iDb < len(db.Databases) );
  zDb = db.Databases[iDb].Name;
  rcauth = pParse.AuthCheck(SQLITE_DELETE, pTab.Name, 0, zDb)
  assert( rcauth==SQLITE_OK || rcauth==SQLITE_DENY || rcauth==SQLITE_IGNORE );
  if( rcauth==SQLITE_DENY ){
    goto delete_from_cleanup;
  }
  assert(!isView || pTrigger);

  /* Assign  cursor number to the table and all its indices.
  */
  assert( pTabList.nSrc==1 );
  iCur = pTabList.a[0].iCursor = pParse.nTab++;
  for _, pIdx := range pTab.Indices {
    pParse.nTab++
  }

	//	Start the view context
	if isView {
		pParse.AuthContextPush(&sContext, pTab.Name)
	}

  /* Begin generating code.
  */
  v = pParse.GetVdbe()
  if( v==0 ){
    goto delete_from_cleanup;
  }
  if( pParse.nested==0 ) sqlite3VdbeCountChanges(v);
  pParse.BeginWriteOperation(1, iDb)

  /* If we are trying to delete from a view, realize that view into
  ** a ephemeral table.
  */
  if( isView ){
    sqlite3MaterializeView(pParse, pTab, pWhere, iCur);
  }

  /* Resolve the column names in the WHERE clause.
  */
  memset(&sNC, 0, sizeof(sNC));
  sNC.Parse = pParse;
  sNC.SrcList = pTabList;
  if( sqlite3ResolveExprNames(&sNC, pWhere) ){
    goto delete_from_cleanup;
  }

  /* Initialize the counter of the number of rows deleted, if
  ** we are counting rows.
  */
  if( db.flags & SQLITE_CountRows ){
    memCnt = ++pParse.nMem;
    v.AddOp2(OP_Integer, 0, memCnt);
  }

#ifndef SQLITE_OMIT_TRUNCATE_OPTIMIZATION
  /* Special case: A DELETE without a WHERE clause deletes everything.
  ** It is easier just to erase the whole table. Prior to version 3.6.5,
  ** this optimization caused the row change count (the value returned by 
  ** API function sqlite3_count_changes) to be set incorrectly.  */
  if( rcauth==SQLITE_OK && pWhere==0 && !pTrigger && !pTab.IsVirtual() 
   && 0==sqlite3FkRequired(pParse, pTab, 0, 0)
  ){
    assert( !isView );
    sqlite3VdbeAddOp4(v, OP_Clear, pTab.tnum, iDb, memCnt, pTab.Name, P4_STATIC);
	for _, pIdx := pTab.Indices {
      assert( pIdx.Schema==pTab.Schema );
      v.AddOp2(OP_Clear, pIdx.tnum, iDb);
    }
  }else
#endif /* SQLITE_OMIT_TRUNCATE_OPTIMIZATION */
  /* The usual case: There is a WHERE clause so we have to scan through
  ** the table and pick which records to delete.
  */
  {
    int iRowSet = ++pParse.nMem;   /* Register for rowset of rows to delete */
    int iRowid = ++pParse.nMem;    /* Used for storing rowid values. */
    int regRowid;                   /* Actual register containing rowids */

    /* Collect rowids of every row to be deleted.
    */
    v.AddOp2(OP_Null, 0, iRowSet);
    pWInfo = pParse.WhereBegin(pTabList, pWhere, nil, nil, WHERE_DUPLICATES_OK)
    if( pWInfo==0 ) goto delete_from_cleanup;
    regRowid = pParse.ExprCodeGetColumn(pTab, -1, iCur, iRowid, 0)
    v.AddOp2(OP_RowSetAdd, iRowSet, regRowid);
    if( db.flags & SQLITE_CountRows ){
      v.AddOp2(OP_AddImm, memCnt, 1);
    }
    sqlite3WhereEnd(pWInfo);

    /* Delete every item whose key was written to the list during the
    ** database scan.  We have to delete items after the scan is complete
    ** because deleting an item can change the scan order.  */
    end = v.MakeLabel()

    /* Unless this is a view, open cursors for the table we are 
    ** deleting from and all its indices. If this is a view, then the
    ** only effect this statement has is to fire the INSTEAD OF 
    ** triggers.  */
    if( !isView ){
      pParse.OpenTableAndIndices(pTab, iCur, OP_OpenWrite)
    }

    addr = v.AddOp3(OP_RowSetRead, iRowSet, end, iRowid);

    /* Delete the row */
    if( pTab.IsVirtual() ){
      const char *pVTab = (const char *)sqlite3GetVTable(db, pTab);
      sqlite3VtabMakeWritable(pParse, pTab);
      sqlite3VdbeAddOp4(v, OP_VUpdate, 0, 1, iRowid, pVTab, P4_VTAB);
      v.ChangeP5(OE_Abort)
      sqlite3MayAbort(pParse);
    }else
    {
      int count = (pParse.nested==0);    /* True to count changes */
      sqlite3GenerateRowDelete(pParse, pTab, iCur, iRowid, count, pTrigger, OE_Default);
    }

    /* End of the delete loop */
    v.AddOp2(OP_Goto, 0, addr);
    v.ResolveLabel(end)

    /* Close the cursors open on the table and its indexes. */
    if( !isView && !pTab.IsVirtual() ){
		i := 1
		for _, pIdx := range pTab.Indices {
			v.AddOp2(OP_Close, iCur + i, pIdx.tnum);
		}
      v.AddOp1(OP_Close, iCur);
    }
  }

  /* Update the sqlite_sequence table by storing the content of the
  ** maximum rowid counter values recorded while inserting into
  ** autoincrement tables.
  */
  if( pParse.nested==0 && pParse.pTriggerTab==0 ){
    sqlite3AutoincrementEnd(pParse);
  }

  /* Return the number of rows that were deleted. If this routine is 
  ** generating code because of a call to sqlite3NestedParse(), do not
  ** invoke the callback function.
  */
  if( (db.flags&SQLITE_CountRows) && !pParse.nested && !pParse.pTriggerTab ){
    v.AddOp2(OP_ResultRow, memCnt, 1);
    sqlite3VdbeSetNumCols(v, 1);
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "rows deleted", SQLITE_STATIC);
  }

delete_from_cleanup:
  sContext.Pop()
  sqlite3SrcListDelete(db, pTabList);
  db.ExprDelete(pWhere)
  return;
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

/*
** This routine generates VDBE code that causes a single row of a
** single table to be deleted.
**
** The VDBE must be in a particular state when this routine is called.
** These are the requirements:
**
**   1.  A read/write cursor pointing to pTab, the table containing the row
**       to be deleted, must be opened as cursor number $iCur.
**
**   2.  Read/write cursors for all indices of pTab must be open as
**       cursor number base+i for the i-th index.
**
**   3.  The record number of the row to be deleted must be stored in
**       memory cell iRowid.
**
** This routine generates code to remove both the table record and all 
** index entries that point to that record.
*/
 void sqlite3GenerateRowDelete(
  Parse *pParse,     /* Parsing context */
  Table *pTab,       /* Table containing the row to be deleted */
  int iCur,          /* Cursor number for the table */
  int iRowid,        /* Memory cell that contains the rowid to delete */
  int count,         /* If non-zero, increment the row change counter */
  Trigger *pTrigger, /* List of triggers to (potentially) fire */
  int onconf         /* Default ON CONFLICT policy for triggers */
){
  Vdbe *v = pParse.pVdbe;        /* Vdbe */
  int iOld = 0;                   /* First register in OLD.* array */
  int iLabel;                     /* Label resolved to end of generated code */

  /* Vdbe is guaranteed to have been allocated by this stage. */
  assert( v );

  /* Seek cursor iCur to the row to delete. If this row no longer exists 
  ** (this can happen if a trigger program has already deleted it), do
  ** not attempt to delete it or fire any DELETE triggers.  */
  iLabel = v.MakeLabel()
  v.AddOp3(OP_NotExists, iCur, iLabel, iRowid);
 
  /* If there are any triggers to fire, allocate a range of registers to
  ** use for the old.* references in the triggers.  */
  if( sqlite3FkRequired(pParse, pTab, 0, 0) || pTrigger ){
    uint32 mask;                     /* Mask of OLD.* columns in use */
    int iCol;                     /* Iterator used while populating OLD.* */

    /* TODO: Could use temporary registers here. Also could attempt to
    ** avoid copying the contents of the rowid register.  */
    mask = sqlite3TriggerColmask(
        pParse, pTrigger, 0, 0, TRIGGER_BEFORE|TRIGGER_AFTER, pTab, onconf
    );
    mask |= pParse.FkOldmask(pTab)
    iOld = pParse.nMem+1;
    pParse.nMem += (1 + pTab.nCol);

    /* Populate the OLD.* pseudo-table register array. These values will be 
    ** used by any BEFORE and AFTER triggers that exist.  */
    v.AddOp2(OP_Copy, iRowid, iOld);
    for(iCol=0; iCol<pTab.nCol; iCol++){
      if( mask==0xffffffff || mask&(1<<iCol) ){
        v.ExprCodeGetColumnOfTable(pTab, iCur, iCol, iOld + iCol + 1)
      }
    }

    /* Invoke BEFORE DELETE trigger programs. */
    sqlite3CodeRowTrigger(pParse, pTrigger, 
        TK_DELETE, 0, TRIGGER_BEFORE, pTab, iOld, onconf, iLabel
    );

    /* Seek the cursor to the row to be deleted again. It may be that
    ** the BEFORE triggers coded above have already removed the row
    ** being deleted. Do not attempt to delete the row a second time, and 
    ** do not fire AFTER triggers.  */
    v.AddOp3(OP_NotExists, iCur, iLabel, iRowid);

    /* Do FK processing. This call checks that any FK constraints that
    ** refer to this table (i.e. constraints attached to other tables) 
    ** are not violated by deleting this row.  */
    pParse.FkCheck(pTab, iOld, 0)
  }

  /* Delete the index and table entries. Skip this step if pTab is really
  ** a view (in which case the only effect of the DELETE statement is to
  ** fire the INSTEAD OF triggers).  */ 
  if( pTab.Select==0 ){
    pParse.GenerateRowIndexDelete(pTab, iCur, 0)
    v.AddOp2(OP_Delete, iCur, (count?OPFLAG_NCHANGE:0));
    if( count ){
      sqlite3VdbeChangeP4(v, -1, pTab.Name, P4_TRANSIENT);
    }
  }

  /* Do any ON CASCADE, SET NULL or SET DEFAULT operations required to
  ** handle rows (possibly in other tables) that refer via a foreign key
  ** to the row just deleted. */ 
  sqlite3FkActions(pParse, pTab, 0, iOld);

  /* Invoke AFTER DELETE trigger programs. */
  sqlite3CodeRowTrigger(pParse, pTrigger, 
      TK_DELETE, 0, TRIGGER_AFTER, pTab, iOld, onconf, iLabel
  );

  /* Jump here if the row had already been deleted before any BEFORE
  ** trigger programs were invoked. Or if a trigger program throws a 
  ** RAISE(IGNORE) exception.  */
  v.ResolveLabel(iLabel);
}

//	This routine generates VDBE code that causes the deletion of all index entries associated with a single row of a single table.
//	The VDBE must be in a particular state when this routine is called. These are the requirements:
//		1.  A read/write cursor pointing to pTab, the table containing the row to be deleted, must be opened as cursor number "iCur".
//		2.  Read/write cursors for all indices of pTab must be open as cursor number iCur+i for the i-th index.
//		3.  The "iCur" cursor must be pointing to the row that is to be deleted.
func (pParse *Parse) GenerateRowIndexDelete(table *Table, iCur int, aRegIdx []int) {
	if len(aRegIdx) > 0 {
		for i, pIdx := range table.Indices {
			if aRegIdx[i] != 0 {
				r1 := sqlite3GenerateIndexKey(pParse, pIdx, iCur, 0, 0)
				i++
				pParse.pVdbe.AddOp3(OP_IdxDelete, iCur + i, r1, len(pIdx.Columns) + 1)
			}
		}
	}
}

//	Generate code that will assemble an index key and put it in register regOut. The key with be for index pIdx which is an index on pTab. iCur is the index of a cursor open on the pTab table and pointing to the entry that needs indexing.
//	Return a register number which is the first in a block of registers that holds the elements of the index key. The block of registers has already been deallocated by the time this routine returns.
int sqlite3GenerateIndexKey(
  Parse *pParse,     /* Parsing context */
  Index *pIdx,       /* The index for which to generate a key */
  int iCur,          /* Cursor number for the pIdx.pTable table */
  int regOut,        /* Write the new index key to this register */
  int doMakeRec      /* Run the OP_MakeRecord instruction if true */
){
  Vdbe *v = pParse.pVdbe;
  int j;
  Table *pTab = pIdx.pTable;
  int regBase;
  int nCol;

  nCol = len(pIdx.Columns)
  regBase = pParse.GetTempRange(nCol + 1)
  v.AddOp2(OP_Rowid, iCur, regBase+nCol);
  for(j=0; j<nCol; j++){
    int idx = pIdx.Columns[j];
    if( idx==pTab.iPKey ){
      v.AddOp2(OP_SCopy, regBase+nCol, regBase+j);
    }else{
      v.AddOp3(OP_Column, iCur, idx, regBase+j);
      v.ColumnDefault(pTab, idx, -1)
    }
  }
  if( doMakeRec ){
    const char *zAff;
    if pTab.Select {
      zAff = ""
    } else {
      zAff = v.IndexAffinityStr(pIdx)
    }
    v.AddOp3(OP_MakeRecord, regBase, nCol+1, regOut);
    sqlite3VdbeChangeP4(v, -1, zAff, P4_TRANSIENT);
  }
  pParse.ReleaseTempRange(regBase, nCol + 1)
  return regBase;
}
