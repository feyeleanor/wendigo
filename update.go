//	The most recently coded instruction was an OP_Column to retrieve the i-th column of table pTab. This routine sets the P4 parameter of the OP_Column to the default value, if any.
//	The default value of a column is specified by a DEFAULT clause in the column definition. This was either supplied by the user when the table was created, or added later to the table definition by an ALTER TABLE command. If the latter, then the row-records in the table btree on disk may not contain a value for the column and the default value, taken from the P4 parameter of the OP_Column instruction, is returned instead. If the former, then all row-records are guaranteed to include a value for the column and the P4 value is not required.
//	Column definitions created by an ALTER TABLE command may only have literal default values specified: a number, null or a string. (If a more complicated default expression value was provided, it is evaluated when the ALTER TABLE is executed and one of the literal values written into the sqlite_master table.)
//	Therefore, the P4 parameter is only required if the default value for the column is a literal number, string or null. The sqlite3::ValueFromExpr() function is capable of transforming these types of expressions into sqlite3_value objects.
//	If parameter iReg is not negative, code an OP_RealAffinity instruction on register iReg. This is used when an equivalent integer value is stored in place of an 8-byte floating point value in order to save space.
func (v *Vdbe) ColumnDefault(table *Table, i, register int) {
	assert( table != nil )
	if table.Select == nil {
		db := v.DB()
		enc := db.Encoding()
		column := table.Columns[i]
		v.Comment(table.Name, ".", column.Name)
		assert( i < table.nCol )

		if _, pValue := db.ValueFromExpr(column.pDflt, enc, column.affinity); pValue != nil {
			sqlite3VdbeChangeP4(v, -1, (const char *)pValue, P4_MEM)
		}
		if register >= 0 && table.Columns[i].affinity == SQLITE_AFF_REAL {
			v.AddOp1(OP_RealAffinity, register)
		}
	}
}

/*
** Process an UPDATE statement.
**
**   UPDATE OR IGNORE table_wxyz SET a=b, c=d WHERE e<5 AND f NOT NULL;
**          \_______/ \________/     \______/       \________________/
*            onError   pTabList      pChanges             pWhere
*/
 void sqlite3Update(
  Parse *pParse,         /* The parser context */
  SrcList *pTabList,     /* The table in which we should change things */
  ExprList *pChanges,    /* Things to be changed */
  Expr *pWhere,          /* The WHERE clause.  May be null */
  int onError            /* How to handle constraint errors */
){
  int i, j;              /* Loop counters */
  Table *pTab;           /* The table to be updated */
  int addr = 0;          /* VDBE instruction address of the start of the loop */
  WhereInfo *pWInfo;     /* Information about the WHERE clause */
  Vdbe *v;               /* The virtual database engine */
  Index *pIdx;           /* For looping over indices */
  int nIdx;              /* Number of indices that need updating */
  int iCur;              /* VDBE Cursor number of pTab */
  sqlite3 *db;           /* The database structure */
  int *aRegIdx = 0;      /* One register assigned to each index to be updated */
  int *aXRef = 0;        /* aXRef[i] is the index in pChanges.a[] of the
                         ** an expression for the i-th column of the table.
                         ** aXRef[i]==-1 if the i-th column is not changed. */
  int chngRowid;         /* True if the record number is being changed */
  Expr *pRowidExpr = 0;  /* Expression defining the new record number */
  int openAll = 0;       /* True if all indices need to be opened */
  AuthContext sContext;  /* The authorization context */
  NameContext sNC;       /* The name-context to resolve expressions in */
  int iDb;               /* Database containing the table being updated */
  int okOnePass;         /* True for one-pass algorithm without the FIFO */
  int hasFK;             /* True if foreign key processing is required */

  int isView;            /* True when updating a view (INSTEAD OF trigger) */
  Trigger *pTrigger;     /* List of triggers on pTab, if required */
  int tmask;             /* Mask of TRIGGER_BEFORE|TRIGGER_AFTER */
  int newmask;           /* Mask of NEW.* columns accessed by BEFORE triggers */

  /* Register Allocations */
  int regRowCount = 0;   /* A count of rows changed */
  int regOldRowid;       /* The old rowid */
  int regNewRowid;       /* The new rowid */
  int regNew;            /* Content of the NEW.* table in triggers */
  int regOld = 0;        /* Content of OLD.* table in triggers */
  int regRowSet = 0;     /* Rowset of rows to be updated */

  memset(&sContext, 0, sizeof(sContext));
  db = pParse.db;
  if( pParse.nErr || db.mallocFailed ){
    goto update_cleanup;
  }
  assert( pTabList.nSrc==1 );

  //	Locate the table which we want to update. 
  pTab = pParse.SrcListLookup(pTabList)
  if( pTab==0 ) goto update_cleanup;
  iDb = pParse.db.SchemaToIndex(pTab.Schema)

  /* Figure out if we have any triggers and if the table being
  ** updated is a view.
  */
  pTrigger = sqlite3TriggersExist(pParse, pTab, TK_UPDATE, pChanges, &tmask);
  isView = pTab.Select!=0;
  assert( pTrigger || tmask==0 );

  if( sqlite3ViewGetColumnNames(pParse, pTab) ){
    goto update_cleanup;
  }
  if( sqlite3IsReadOnly(pParse, pTab, tmask) ){
    goto update_cleanup;
  }
  aXRef = sqlite3DbMallocRaw(db, sizeof(int) * pTab.nCol );
  if( aXRef==0 ) goto update_cleanup;
  for(i=0; i<pTab.nCol; i++) aXRef[i] = -1;

  /* Allocate a cursors for the main database table and for all indices.
  ** The index cursors might not be used, but if they are used they
  ** need to occur right after the database cursor.  So go ahead and
  ** allocate enough space, just in case.
  */
  iCur = pParse.nTab
  pTabList.a[0].iCursor = iCur
	pParse.nTab += len(pTab.Indices) + 1

  /* Initialize the name-context */
  memset(&sNC, 0, sizeof(sNC));
  sNC.Parse = pParse;
  sNC.pSrcList = pTabList;

  /* Resolve the column names in all the expressions of the
  ** of the UPDATE statement.  Also find the column index
  ** for each column to be updated in the pChanges array.  For each
  ** column to be updated, make sure we have authorization to change
  ** that column.
  */
  chngRowid = 0;
  for(i=0; i<pChanges.nExpr; i++){
    if( sqlite3ResolveExprNames(&sNC, pChanges.a[i].Expr) ){
      goto update_cleanup;
    }
    for(j=0; j<pTab.nCol; j++){
      if !CaseInsensitiveMatch(pTab.Columns[j].Name, pChanges.a[i].Name) {
        if( j==pTab.iPKey ){
          chngRowid = 1;
          pRowidExpr = pChanges.a[i].Expr;
        }
        aXRef[j] = i;
        break;
      }
    }
    if( j>=pTab.nCol ){
      if( sqlite3IsRowid(pChanges.a[i].Name) ){
        chngRowid = 1;
        pRowidExpr = pChanges.a[i].Expr;
      }else{
        pParse.SetErrorMsg("no such column: %v", pChanges.a[i].Name);
        pParse.checkSchema = 1;
        goto update_cleanup;
      }
    }
    {
      switch rc := pParse.AuthCheck(SQLITE_UPDATE, pTab.Name, pTab.Columns[j].Name, db.Databases[iDb].Name); rc {
	  case SQLITE_DENY:
        goto update_cleanup;
      case SQLITE_IGNORE:
        aXRef[j] = -1
      }
    }
  }

  hasFK = sqlite3FkRequired(pParse, pTab, aXRef, chngRowid);

	//	Allocate memory for the array aRegIdx[]. There is one entry in the array for each index associated with table being updated. Fill in the value with a register number for indices that are to be used and with zero for unused indices.
	if nIndex := len(pTab.Indices); nIndex > 0 {
		aRegIdx = make([]*Index, nIndex)
	}
	for _, index := range pTab.Indices {
		reg			int
		if hasFK || chngRowid {
			pParse.nMem++
			reg = pParse.nMem
		} else {
			reg = 0
			range _, column := range index.Columns {
				if aXRef[column] >= 0 {
					pParse.nMem++
					reg = pParse.nMem
					break
				}
			}
		}
		aRegIdx = append(aRegIdx, reg)
	}

  /* Begin generating code. */
  v = pParse.GetVdbe()
  if( v==0 ) goto update_cleanup;
  if( pParse.nested==0 ) sqlite3VdbeCountChanges(v);
  pParse.BeginWriteOperation(1, iDb)

  /* Virtual tables must be handled separately */
  if( pTab.IsVirtual() ){
    updateVirtualTable(pParse, pTabList, pTab, pChanges, pRowidExpr, aXRef, pWhere, onError);
    pWhere = 0;
    pTabList = 0;
    goto update_cleanup;
  }

  /* Allocate required registers. */
  regRowSet = ++pParse.nMem;
  regOldRowid = regNewRowid = ++pParse.nMem;
  if( pTrigger || hasFK ){
    regOld = pParse.nMem + 1;
    pParse.nMem += pTab.nCol;
  }
  if( chngRowid || pTrigger || hasFK ){
    regNewRowid = ++pParse.nMem;
  }
  regNew = pParse.nMem + 1;
  pParse.nMem += pTab.nCol;

	//	Start the view context.
	if isView {
		pParse.AuthContextPush(&sContext, pTab.Name)
	}

  /* If we are trying to update a view, realize that view into
  ** a ephemeral table.
  */
  if( isView ){
    sqlite3MaterializeView(pParse, pTab, pWhere, iCur);
  }

  /* Resolve the column names in all the expressions in the
  ** WHERE clause.
  */
  if( sqlite3ResolveExprNames(&sNC, pWhere) ){
    goto update_cleanup;
  }

  /* Begin the database scan
  */
  v.AddOp3(OP_Null, 0, regRowSet, regOldRowid);
  pWInfo = pParse.WhereBegin(pTabList, pWhere, nil, nil, WHERE_ONEPASS_DESIRED)
  if( pWInfo==0 ) goto update_cleanup;
  okOnePass = pWInfo.okOnePass;

  /* Remember the rowid of every item to be updated.
  */
  v.AddOp2(OP_Rowid, iCur, regOldRowid);
  if( !okOnePass ){
    v.AddOp2(OP_RowSetAdd, regRowSet, regOldRowid);
  }

  /* End the database scan loop.
  */
  sqlite3WhereEnd(pWInfo);

	//	Initialize the count of updated rows
	if (db.flags & SQLITE_CountRows) && !pParse.pTriggerTab {
		pParse.nMem++
		regRowCount = pParse.nMem
		v.AddOp2(OP_Integer, 0, regRowCount)
	}

	if !isView {
		//	Open every index that needs updating. Note that if any index could potentially invoke a REPLACE conflict resolution action, then we need to open all indices because we might need to be deleting some records.
		if !okOnePass {
			pParse.OpenTable(pTab, iCur, iDb, OP_OpenWrite)
		}
		if onError == OE_Replace {
			openAll = true
		} else {
			openAll = false
			for _, index := range pTab.Indices {
				if index.onError == OE_Replace {
					openAll = true
					break
				}
			}
		}

		for i, index := range pTab.Indices {
			assert( aRegIdx )
			if openAll || aRegIdx[i] > 0 {
				pKey := pParse.IndexKeyinfo(index)
				sqlite3VdbeAddOp4(v, OP_OpenWrite, iCur + i + 1, index.tnum, iDb, (char*)pKey, P4_KEYINFO_HANDOFF)
				assert( pParse.nTab > iCur + i + 1 )
			}
		}
	}

  /* Top of the update loop */
  if( okOnePass ){
    int a1 = v.AddOp1(OP_NotNull, regOldRowid);
    addr = v.AddOp0(OP_Goto);
    v.JumpHere(a1)
  }else{
    addr = v.AddOp3(OP_RowSetRead, regRowSet, 0, regOldRowid);
  }

  /* Make cursor iCur point to the record that is being updated. If
  ** this record does not exist for some reason (deleted by a trigger,
  ** for example, then jump to the next iteration of the RowSet loop.  */
  v.AddOp3(OP_NotExists, iCur, addr, regOldRowid);

  /* If the record number will change, set register regNewRowid to
  ** contain the new value. If the record number is not being modified,
  ** then regNewRowid is the same register as regOldRowid, which is
  ** already populated.  */
  assert( chngRowid || pTrigger || hasFK || regOldRowid==regNewRowid );
  if( chngRowid ){
    sqlite3ExprCode(pParse, pRowidExpr, regNewRowid);
    v.AddOp1(OP_MustBeInt, regNewRowid);
  }

  /* If there are triggers on this table, populate an array of registers 
  ** with the required old.* column data.  */
  if( hasFK || pTrigger ){
	oldmask		uint32
	if hasFK {
		oldmask = pParse.FkOldmask(pTab)
	}
    oldmask |= sqlite3TriggerColmask(pParse, pTrigger, pChanges, 0, TRIGGER_BEFORE|TRIGGER_AFTER, pTab, onError);
    for(i=0; i<pTab.nCol; i++){
      if( aXRef[i]<0 || oldmask==0xffffffff || (i<32 && (oldmask & (1<<i))) ){
        v.ExprCodeGetColumnOfTable(pTab, iCur, i, regOld+i)
      }else{
        v.AddOp2(OP_Null, 0, regOld+i);
      }
    }
    if( chngRowid==0 ){
      v.AddOp2(OP_Copy, regOldRowid, regNewRowid);
    }
  }

  /* Populate the array of registers beginning at regNew with the new
  ** row data. This array is used to check constaints, create the new
  ** table and index records, and as the values for any new.* references
  ** made by triggers.
  **
  ** If there are one or more BEFORE triggers, then do not populate the
  ** registers associated with columns that are (a) not modified by
  ** this UPDATE statement and (b) not accessed by new.* references. The
  ** values for registers not modified by the UPDATE must be reloaded from 
  ** the database after the BEFORE triggers are fired anyway (as the trigger 
  ** may have modified them). So not loading those that are not going to
  ** be used eliminates some redundant opcodes.
  */
  newmask = sqlite3TriggerColmask(
      pParse, pTrigger, pChanges, 1, TRIGGER_BEFORE, pTab, onError
  );
  v.AddOp3(OP_Null, 0, regNew, regNew+pTab.nCol-1);
  for(i=0; i<pTab.nCol; i++){
    if( i==pTab.iPKey ){
      /*v.AddOp2(OP_Null, 0, regNew+i);*/
    }else{
      j = aXRef[i];
      if( j>=0 ){
        sqlite3ExprCode(pParse, pChanges.a[j].Expr, regNew+i);
      }else if( 0==(tmask&TRIGGER_BEFORE) || i>31 || (newmask&(1<<i)) ){
        /* This branch loads the value of a column that will not be changed 
        ** into a register. This is done if there are no BEFORE triggers, or
        ** if there are one or more BEFORE triggers that use this value via
        ** a new.* reference in a trigger program.
        */
        v.AddOp3(OP_Column, iCur, i, regNew+i);
        v.ColumnDefault(pTab, i, regNew + i)
      }
    }
  }

  /* Fire any BEFORE UPDATE triggers. This happens before constraints are
  ** verified. One could argue that this is wrong.
  */
  if( tmask&TRIGGER_BEFORE ){
    v.AddOp2(OP_Affinity, regNew, pTab.nCol);
    sqlite3TableAffinityStr(v, pTab);
    sqlite3CodeRowTrigger(pParse, pTrigger, TK_UPDATE, pChanges, 
        TRIGGER_BEFORE, pTab, regOldRowid, onError, addr);

    /* The row-trigger may have deleted the row being updated. In this
    ** case, jump to the next row. No updates or AFTER triggers are 
    ** required. This behaviour - what happens when the row being updated
    ** is deleted or renamed by a BEFORE trigger - is left undefined in the
    ** documentation.
    */
    v.AddOp3(OP_NotExists, iCur, addr, regOldRowid);

    /* If it did not delete it, the row-trigger may still have modified 
    ** some of the columns of the row being updated. Load the values for 
    ** all columns not modified by the update statement into their 
    ** registers in case this has happened.
    */
    for(i=0; i<pTab.nCol; i++){
      if( aXRef[i]<0 && i!=pTab.iPKey ){
        v.AddOp3(OP_Column, iCur, i, regNew+i);
        v.ColumnDefault(pTab, i, regNew + i)
      }
    }
  }

  if( !isView ){
    int j1;                       /* Address of jump instruction */

    /* Do constraint checks. */
    sqlite3GenerateConstraintChecks(pParse, pTab, iCur, regNewRowid,
        aRegIdx, (chngRowid?regOldRowid:0), 1, onError, addr, 0);

    /* Do FK constraint checks. */
    if( hasFK ){
      pParse.FkCheck(pTab, regOldRowid, 0)
    }

    /* Delete the index entries associated with the current record.  */
    j1 = v.AddOp3(OP_NotExists, iCur, 0, regOldRowid);
    pParse.GenerateRowIndexDelete(pTab, iCur, aRegIdx)
  
    /* If changing the record number, delete the old record.  */
    if( hasFK || chngRowid ){
      v.AddOp2(OP_Delete, iCur, 0);
    }
    v.JumpHere(j1)

    if( hasFK ){
      pParse.FkCheck(pTab, 0, regNewRowid)
    }
  
    //	Insert the new index entries and the new record.
    pParse.CompleteInsertion(pTab, iCur, regNewRowid, aRegIdx, 1, 0, 0)

    //	Do any ON CASCADE, SET NULL or SET DEFAULT operations required to handle rows (possibly in other tables) that refer via a foreign key to the row just updated.
    if( hasFK ){
      sqlite3FkActions(pParse, pTab, pChanges, regOldRowid);
    }
  }

  /* Increment the row counter 
  */
  if( (db.flags & SQLITE_CountRows) && !pParse.pTriggerTab){
    v.AddOp2(OP_AddImm, regRowCount, 1);
  }

  sqlite3CodeRowTrigger(pParse, pTrigger, TK_UPDATE, pChanges, TRIGGER_AFTER, pTab, regOldRowid, onError, addr);

  /* Repeat the above with the next record to be updated, until
  ** all record selected by the WHERE clause have been updated.
  */
  v.AddOp2(OP_Goto, 0, addr);
  v.JumpHere(addr)

	//	Close all tables
	for i, _ := range pTab.Indices {
		assert( aRegIdx )
		if openAll || aRegIdx[i] > 0 {
			v.AddOp2(OP_Close, iCur + i + 1, 0)
		}
	}
	v.AddOp2(OP_Close, iCur, 0)

	//	Update the sqlite_sequence table by storing the content of the maximum rowid counter values recorded while inserting into autoincrement tables.
	if !pParse.nested && pParse.pTriggerTab == nil {
		sqlite3AutoincrementEnd(pParse)
	}

	//	Return the number of rows that were changed. If this routine is generating code because of a call to sqlite3NestedParse(), do not invoke the callback function.
	if (db.flags & SQLITE_CountRows) && !pParse.pTriggerTab && !pParse.nested {
		v.AddOp2(OP_ResultRow, regRowCount, 1)
		sqlite3VdbeSetNumCols(v, 1)
		sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "rows updated", SQLITE_STATIC)
	}

update_cleanup:
	sContext.Pop()
	aRegIdx = nil
	aXRef = nil
	sqlite3SrcListDelete(db, pTabList)
	db.ExprListDelete(pChanges)
	db.ExprDelete(pWhere)
	return
}
/* Make sure "isView" and other macros defined above are undefined. Otherwise
** thely may interfere with compilation of other functions in this file
** (or in another file, if this file becomes part of the amalgamation).  */
#ifdef isView
 #undef isView
#endif

/*
** Generate code for an UPDATE of a virtual table.
**
** The strategy is that we create an ephemerial table that contains
** for each row to be changed:
**
**   (A)  The original rowid of that row.
**   (B)  The revised rowid for the row. (note1)
**   (C)  The content of every column in the row.
**
** Then we loop over this ephemeral table and for each row in
** the ephermeral table call VUpdate.
**
** When finished, drop the ephemeral table.
**
** (note1) Actually, if we know in advance that (A) is always the same
** as (B) we only store (A), then duplicate (A) when pulling
** it out of the ephemeral table before calling VUpdate.
*/
static void updateVirtualTable(
  Parse *pParse,       /* The parsing context */
  SrcList *pSrc,       /* The virtual table to be modified */
  Table *pTab,         /* The virtual table */
  ExprList *pChanges,  /* The columns to change in the UPDATE statement */
  Expr *pRowid,        /* Expression used to recompute the rowid */
  int *aXRef,          /* Mapping from columns of pTab to entries in pChanges */
  Expr *pWhere,        /* WHERE clause of the UPDATE statement */
  int onError          /* ON CONFLICT strategy */
){
  Vdbe *v = pParse.pVdbe;  /* Virtual machine under construction */

  Select *pSelect = 0;      /* The SELECT statement */
  Expr *pExpr;              /* Temporary expression */
  int ephemTab;             /* Table holding the result of the SELECT */
  int i;                    /* Loop counter */
  int addr;                 /* Address of top of loop */
  int iReg;                 /* First register in set passed to OP_VUpdate */
  sqlite3 *db = pParse.db; /* Database connection */
  const char *pVTab = (const char*)sqlite3GetVTable(db, pTab);
  SelectDest dest;

  /* Construct the SELECT statement that will find the new values for
  ** all updated rows. 
  */
  pEList := NewExprList(db.Expr(TK_ID, "_rowid_"))
  if( pRowid ){
    pEList = append(pEList.Items, pRowid.Dup())
  }
  assert( pTab.iPKey<0 );
  for(i=0; i<pTab.nCol; i++){
    if( aXRef[i]>=0 ){
      pExpr = pChanges.a[aXRef[i]].Expr.Dup();
    }else{
      pExpr = db.Expr(TK_ID, pTab.Columns[i].Name)
    }
    pEList = append(pEList.Items, pExpr)
  }
  pSelect = sqlite3SelectNew(pParse, pEList, pSrc, pWhere, 0, 0, 0, 0, 0, 0);
  
  /* Create the ephemeral table into which the update results will
  ** be stored.
  */
  assert( v );
  ephemTab = pParse.nTab++;
  v.AddOp2(OP_OpenEphemeral, ephemTab, pTab.nCol+1+(pRowid!=0));
  v.ChangeP5(BTREE_UNORDERED)

  /* fill the ephemeral table 
  */
  sqlite3SelectDestInit(&dest, SRT_Table, ephemTab);
  sqlite3Select(pParse, pSelect, &dest);

  /* Generate code to scan the ephemeral table and call VUpdate. */
  iReg = ++pParse.nMem;
  pParse.nMem += pTab.nCol+1;
  addr = v.AddOp2(OP_Rewind, ephemTab, 0);
  v.AddOp3(OP_Column,  ephemTab, 0, iReg);
  v.AddOp3(OP_Column, ephemTab, (pRowid?1:0), iReg+1);
  for(i=0; i<pTab.nCol; i++){
    v.AddOp3(OP_Column, ephemTab, i+1+(pRowid!=0), iReg+2+i);
  }
  sqlite3VtabMakeWritable(pParse, pTab);
  sqlite3VdbeAddOp4(v, OP_VUpdate, 0, pTab.nCol+2, iReg, pVTab, P4_VTAB);
  v.ChangeP5(onError == OE_Default ? OE_Abort : onError)
  sqlite3MayAbort(pParse);
  v.AddOp2(OP_Next, ephemTab, addr+1);
  v.JumpHere(addr)
  v.AddOp2(OP_Close, ephemTab, 0);

  /* Cleanup */
  sqlite3SelectDelete(db, pSelect);  
}
