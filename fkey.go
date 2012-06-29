//	This file contains code used by the compiler to add foreign key support to compiled SQL statements.

/*
** Deferred and Immediate FKs
** --------------------------
**
** Foreign keys in SQLite come in two flavours: deferred and immediate.
** If an immediate foreign key constraint is violated, SQLITE_CONSTRAINT
** is returned and the current statement transaction rolled back. If a 
** deferred foreign key constraint is violated, no action is taken 
** immediately. However if the application attempts to commit the 
** transaction before fixing the constraint violation, the attempt fails.
**
** Deferred constraints are implemented using a simple counter associated
** with the database handle. The counter is set to zero each time a 
** database transaction is opened. Each time a statement is executed 
** that causes a foreign key violation, the counter is incremented. Each
** time a statement is executed that removes an existing violation from
** the database, the counter is decremented. When the transaction is
** committed, the commit fails if the current value of the counter is
** greater than zero. This scheme has two big drawbacks:
**
**   * When a commit fails due to a deferred foreign key constraint, 
**     there is no way to tell which foreign constraint is not satisfied,
**     or which row it is not satisfied for.
**
**   * If the database contains foreign key violations when the 
**     transaction is opened, this may cause the mechanism to malfunction.
**
** Despite these problems, this approach is adopted as it seems simpler
** than the alternatives.
**
** INSERT operations:
**
**   I.1) For each FK for which the table is the child table, search
**        the parent table for a match. If none is found increment the
**        constraint counter.
**
**   I.2) For each FK for which the table is the parent table, 
**        search the child table for rows that correspond to the new
**        row in the parent table. Decrement the counter for each row
**        found (as the constraint is now satisfied).
**
** DELETE operations:
**
**   D.1) For each FK for which the table is the child table, 
**        search the parent table for a row that corresponds to the 
**        deleted row in the child table. If such a row is not found, 
**        decrement the counter.
**
**   D.2) For each FK for which the table is the parent table, search 
**        the child table for rows that correspond to the deleted row 
**        in the parent table. For each found increment the counter.
**
** UPDATE operations:
**
**   An UPDATE command requires that all 4 steps above are taken, but only
**   for FK constraints for which the affected columns are actually 
**   modified (values must be compared at runtime).
**
** Note that I.1 and D.1 are very similar operations, as are I.2 and D.2.
** This simplifies the implementation a bit.
**
** For the purposes of immediate FK constraints, the OR REPLACE conflict
** resolution is considered to delete rows before the new row is inserted.
** If a delete caused by OR REPLACE violates an FK constraint, an exception
** is thrown, even if the FK constraint would be satisfied after the new 
** row is inserted.
**
** Immediate constraints are usually handled similarly. The only difference 
** is that the counter used is stored as part of each individual statement
** object (struct Vdbe). If, after the statement has run, its immediate
** constraint counter is greater than zero, it returns SQLITE_CONSTRAINT
** and the statement transaction is rolled back. An exception is an INSERT
** statement that inserts a single row only (no triggers). In this case,
** instead of using a counter, an exception is thrown immediately if the
** INSERT violates a foreign key constraint. This is necessary as such
** an INSERT does not open a statement transaction.
**
** TODO: How should dropping a table be handled? How should renaming a 
** table be handled?
**
**
** Query API Notes
** ---------------
**
** Before coding an UPDATE or DELETE row operation, the code-generator
** for those two operations needs to know whether or not the operation
** requires any FK processing and, if so, which columns of the original
** row are required by the FK processing VDBE code (i.e. if FKs were
** implemented using triggers, which of the old.* columns would be 
** accessed). No information is required by the code-generator before
** coding an INSERT operation. The functions used by the UPDATE/DELETE
** generation code to query for this information are:
**
**   sqlite3FkRequired() - Test to see if FK processing is required.
**   FkOldmask()  - Query for the set of required old.* columns.
**
**
** Externally accessible module functions
** --------------------------------------
**
**   FkCheck()    - Check for foreign key violations.
**   sqlite3FkActions()  - Code triggers for ON UPDATE/ON DELETE actions.
**   sqlite3FkDelete()   - Delete an ForeignKey structure.
*/

/*
** VDBE Calling Convention
** -----------------------
**
** Example:
**
**   For the following INSERT statement:
**
**     CREATE TABLE t1(a, b INTEGER PRIMARY KEY, c);
**     INSERT INTO t1 VALUES(1, 2, 3.1);
**
**   Register (x):        2    (type integer)
**   Register (x+1):      1    (type integer)
**   Register (x+2):      NULL (type NULL)
**   Register (x+3):      3.1  (type real)
*/

//	A foreign key constraint requires that the key columns in the parent table are collectively subject to a UNIQUE or PRIMARY KEY constraint. Given that pParent is the parent table for foreign key constraint key, search the schema for a unique index on the parent key columns.
//	If successful, zero is returned. If the parent key is an INTEGER PRIMARY KEY column, then output variable index is set to nil. Otherwise, index is set to point to the unique index. 
//	If the parent key consists of a single column (the foreign key constraint is not a composite foreign key), output variable columns is set to NULL. Otherwise, it is set to point to an allocated array of size N, where N is the number of columns in the parent key. The first element of the array is the index of the child table column that is mapped by the FK constraint to the parent table column stored in the left-most column of index. The second element of the array is the index of the child table column that corresponds to the second left-most column of index, and so on.
//	If the required index cannot be found, either because:
//		1) The named parent key columns do not exist, or
//		2) The named parent key columns do exist, but are not subject to a UNIQUE or PRIMARY KEY constraint, or
//		3) No parent key columns were provided explicitly as part of the foreign key definition, and the parent table does not have a PRIMARY KEY, or
//		4) No parent key columns were provided explicitly as part of the foreign key definition, and the PRIMARY KEY of the parent table consists of a a different number of columns to the child key in the child table.
//	then non-zero is returned, and a "foreign key mismatch" error loaded into pParse. If an OOM error occurs, non-zero is returned and the pParse.db.mallocFailed flag is set.

func (p *Parse) LocateFkeyIndex(pParent *Table, key *ForeignKey) (index *Index, columns []int, rc int) {
	zKey := key.Columns[0].zCol				//	Name of left-most parent key column

	//	If this is a non-composite (single column) foreign key, check if it maps to the INTEGER PRIMARY KEY of table pParent. If so, leave index and columns set to nil and return early. 
	//	Otherwise, for a composite foreign key (more than one column), allocate space for the aiCol array (returned via output parameter *paiCol). Non-composite foreign keys do not require the aiCol array.
	nCol := key.nCol
	if nCol == 1 {
		//	The FK maps to the IPK if any of the following are true:
		//		1) There is an INTEGER PRIMARY KEY column and the FK is implicitly mapped to the primary key of table pParent, or
		//		2) The FK is explicitly mapped to a column declared as INTEGER PRIMARY KEY.
		if pParent.iPKey >= 0 {
			if zKey == "" || CaseInsensitiveMatch(pParent.Columns[pParent.iPKey].Name, zKey) {
				rc = SQLITE_OK
				return
			}
		}
	} else {
		assert( nCol > 1 )
		columns = make([]int, nCol)
	}

	for _, index = range pParent.Indices {
		if len(index.Columns) == nCol && index.onError != OE_None {
			//	index is a UNIQUE index (or a PRIMARY KEY) and has the right number of columns. If each indexed column corresponds to a foreign key column of ForeignKey, then this index is a winner.
			if zKey == "" {
				//	If zKey is NULL, then this foreign key is implicitly mapped to the PRIMARY KEY of table pParent. The PRIMARY KEY index may be identified by the test (Index.autoIndex == 2).
				if index.autoIndex == 2 {
					if columns != nil {
						for i := 0; i < nCol; i++ {
							columns[i] = key.Columns[i].iFrom
						}
					}
					break
				}
			} else {
				//	If zKey is non-NULL, then this foreign key was declared to map to an explicit list of columns in table pParent. Check if this index matches those columns. Also, check that the index uses the default collation sequences for each column.
				i, j	int
				for i = 0; i < nCol; i++ {
					iCol = index.Columns[i]
					//	If the index uses a collation sequence that is different from the default collation sequence for the column, this index is unusable. Bail out early in this case.
					if zDfltColl = pParent.Columns[iCol].zColl; zDfltColl == nil {
						zDfltColl = "BINARY"
					}
					if !CaseInsensitiveMatch(index.azColl[i], zDfltColl) {
						break
					}

					for j = 0; j < nCol; j++ {
						if CaseInsensitiveMatch(key.Columns[j].zCol, pParent.Columns[iCol].Name) {
							if columns != nil {
								columns[i] = key.Columns[j].iFrom
							}
							break
						}
					}
					if j == nCol {
						break
					}
				}
				if i == nCol {
					break      /* index is usable */
				}
			}
		}
	}

	if index == nil {
		if !p.disableTriggers {
			p.SetErrorMsg("foreign key mismatch")
		}
		columns = nil
		return SQLITE_ERROR
	}
	return 0
}

//	This function is called when a row is inserted into or deleted from the child table of foreign key constraint ForeignKey. If an SQL UPDATE is executed on the child table of ForeignKey, this function is invoked twice for each row affected - once to "delete" the old row, and then again to "insert" the new row.
//	Each time it is called, this function generates VDBE code to locate the row in the parent table that corresponds to the row being inserted into or deleted from the child table. If the parent row can be found, no special action is taken. Otherwise, if the parent row can *not* be found in the parent table:
//			Operation | FK type   | Action taken
//			--------------------------------------------------------------------------
//			INSERT      immediate   Increment the "immediate constraint counter".
//			DELETE      immediate   Decrement the "immediate constraint counter".
//			INSERT      deferred    Increment the "deferred constraint counter".
//			DELETE      deferred    Decrement the "deferred constraint counter".
//	These operations are identified in the comment at the top of this file (fkey.c) as "I.1" and "D.1".
func (pParse *Parse) fkLookupParent(iDb int, table *Table, index *Index, key *ForeignKey, columns []int, child_row_address, constraint_increment int, all_null bool) {
	v := pParse.GetVdbe()
	cursor := pParse.nTab - 1
	KEY_FOUND := v.MakeLabel()			//	jump here if parent key found

	//	If constraint_increment is less than zero, then check at runtime if there are any outstanding constraints to resolve. If there are not, there is no need to check if deleting this row resolves any outstanding violations.
	//	Check if any of the key columns in the child table row are nil. If any are, then the constraint is considered satisfied. No need to search for a matching row in the parent table.
	if constraint_increment < 0 {
		v.AddOp2(OP_FkIfZero, key.isDeferred, KEY_FOUND)
	}
	for i := 0; i < key.nCol; i++ {
		v.AddOp2(OP_IsNull, (columns[i] + child_row_address + 1), KEY_FOUND)
	}

	if !all_null {
		if index == nil {
			//	If index is nil, then the parent key is the INTEGER PRIMARY KEY column of the parent table (table pTab).
			regTemp := pParse.GetTempReg()  
			//	Invoke MustBeInt to coerce the child key value to an integer (i.e. apply the affinity of the parent key). If this fails, then there is no matching parent key. Before using MustBeInt, make a copy of the value. Otherwise, the value inserted into the child key column will have INTEGER affinity applied to it, which may not be correct.
			v.AddOp2(OP_SCopy, (columns[0] + 1 + child_row_address), regTemp)
			iMustBeInt := v.AddOp2(OP_MustBeInt, regTemp, 0)					//	Address of MustBeInt instruction
  
			//	If the parent table is the same as the child table, and we are about to increment the constraint-counter (i.e. this is an INSERT operation), then check if the row being inserted matches itself. If so, do not increment the constraint-counter.
			if table == key.pFrom && constraint_increment == 1 {
				v.AddOp3(OP_Eq, child_row_address, KEY_FOUND, regTemp)
			}
  
			pParse.OpenTable(table, cursor, iDb, OP_OpenRead)
			v.AddOp3(OP_NotExists, cursor, 0, regTemp)
			v.AddOp2(OP_Goto, 0, KEY_FOUND)
			v.JumpHere(v.CurrentAddr() - 2)
			v.JumpHere(iMustBeInt)
			pParse.ReleaseTempReg(regTemp)
		} else {
			nCol := key.nCol
			regTemp := pParse.GetTempRange(nCol)
			regRec := pParse.GetTempReg()
			pKey := pParse.IndexKeyinfo(index)
			v.AddOp3(OP_OpenRead, cursor, index.tnum, iDb)
			sqlite3VdbeChangeP4(v, -1, (char*)pKey, P4_KEYINFO_HANDOFF)
			for i := 0; i < nCol; i++ {
				v.AddOp2(OP_Copy, (columns[i] + 1 + child_row_address), regTemp + i)
			}
  
			//	If the parent table is the same as the child table, and we are about to increment the constraint-counter (i.e. this is an INSERT operation), then check if the row being inserted matches itself. If so, do not increment the constraint-counter.
			//	If any of the parent-key values are nil, then the row cannot match itself. So set JUMPIFNULL to make sure we do the OP_Found if any of the parent-key values are nil (at this point it is known that none of the child key values are).
			if table == key.pFrom && constraint_increment == 1 {
				iJump := v.CurrentAddr() + nCol + 1
				for i := 0; i < nCol; i++ {
					iChild := columns[i] + 1 + child_row_address
					iParent := index.Columns[i] + 1 + child_row_address
					assert( columns[i] != table.iPKey )
					if index.Columns[i] == table.iPKey {
						//	The parent key is a composite key that includes the IPK column
						iParent = child_row_address
					}
					v.AddOp3(OP_Ne, iChild, iJump, iParent)
					v.ChangeP5(SQLITE_JUMPIFNULL)
				}
				v.AddOp2(OP_Goto, 0, KEY_FOUND)
			}
			v.AddOp3(OP_MakeRecord, regTemp, nCol, regRec)
			sqlite3VdbeChangeP4(v, -1, v.IndexAffinityStr(index), P4_TRANSIENT)
			sqlite3VdbeAddOp4Int(v, OP_Found, cursor, KEY_FOUND, regRec, 0)
			pParse.ReleaseTempReg(regRec)
			pParse.ReleaseTempRange(regTemp, nCol)
		}
	}

	if !key.isDeferred && !pParse.pToplevel && !pParse.isMultiWrite {
		//	Special case: If this is an INSERT statement that will insert exactly one row into the table, raise a constraint immediately instead of incrementing a counter. This is necessary as the VM code is being generated for will not open a statement transaction.
		assert( constraint_increment == 1 )
		sqlite3HaltConstraint(pParse, OE_Abort, "foreign key constraint failed", P4_STATIC)
	} else {
		if constraint_increment > 0 && !key.isDeferred {
			pParse.Toplevel().mayAbort = true
		}
		v.AddOp2(OP_FkCounter, key.isDeferred, constraint_increment)
	}
	v.ResolveLabel(KEY_FOUND)
	v.AddOp1(OP_Close, cursor)
}

//	This function is called to generate code executed when a row is deleted from the parent table of foreign key constraint ForeignKey and, if ForeignKey is deferred, when a row is inserted into the same table. When generating code for an SQL UPDATE operation, this function may be called twice - once to "delete" the old row and once to "insert" the new row.
//	The code generated by this function scans through the rows in the child table that correspond to the parent table row being deleted or inserted. For each child row found, one of the following actions is taken:
//			Operation | FK type   | Action taken
//			--------------------------------------------------------------------------
//			DELETE      immediate   Increment the "immediate constraint counter".
//									Or, if the ON (UPDATE|DELETE) action is RESTRICT, throw a "foreign key constraint failed" exception.
//			INSERT      immediate   Decrement the "immediate constraint counter".
//			DELETE      deferred    Increment the "deferred constraint counter".
//									Or, if the ON (UPDATE|DELETE) action is RESTRICT, throw a "foreign key constraint failed" exception.
//			INSERT      deferred    Decrement the "deferred constraint counter".
//	These operations are identified in the comment at the top of this file as "I.2" and "D.2".
func (pParse *Parse) fkScanChildren(sources *SrcList, table *Table, index *Index, key *ForeignKey, columns []int, child_data_address, constraint_increment int) {
	assert( index == nil || index.pTable == table )
	db := pParse.db
	v := pParse.GetVdbe()

	FK_IF_ZERO := 0
	if constraint_increment < 0 {
		FK_IF_ZERO = v.AddOp2(OP_FkIfZero, key.isDeferred, 0)
	}

	//	Create an Expr object representing an SQL expression like:
	//		<parent-key1> = <child-key1> AND <parent-key2> = <child-key2> ...
	//	The collation sequence used for the comparison should be that of the parent key columns. The affinity of the parent key column should be applied to each child key value before the comparison takes place.
    WHERE		*Expr
	for i := 0; i < key.nCol; i++ {
		iCol	int			//	Index of column in child table
		ROW_VALUE := db.Expr(TK_REGISTER, "")
		if ROW_VALUE != nil {
			//	Set the collation sequence and affinity of the LHS of each TK_EQ expression to the parent key column defaults.
			if index != nil {
				Column *pCol
				iCol = index.Columns[i]
				pCol = &table.Columns[iCol]
				if table.iPKey == iCol {
					iCol = -1
				}
				ROW_VALUE.iTable = child_data_address + iCol + 1
				ROW_VALUE.affinity = pCol.affinity
				ROW_VALUE.pColl = pParse.LocateCollSeq(pCol.zColl)
			} else {
				ROW_VALUE.iTable = child_data_address
				ROW_VALUE.affinity = SQLITE_AFF_INTEGER
			}
		}
		if columns != nil {
			iCol = columns[i]
		} else {
			iCol = key.Columns[0].iFrom
		}
		assert( iCol >= 0 )
		CHILD_COLUMN := db.Expr(TK_ID, key.pFrom.Columns[iCol].Name)
		WHERE = db.ExprAnd(WHERE, pParse.Expr(TK_EQ, ROW_VALUE, CHILD_COLUMN, ""))
	}

	//	If the child table is the same as the parent table, and this scan is taking place as part of a DELETE operation (operation D.2), omit the row being deleted from the scan by adding ($rowid != rowid) to the WHERE clause, where $rowid is the rowid of the row being deleted.
	if table == key.pFrom && constraint_increment > 0 {
		ROW_VALUE := db.Expr(TK_REGISTER, "")
		CHILD_COLUMN := db.Expr(TK_COLUMN, "")
		if ROW_VALUE != nil && CHILD_COLUMN != nil {
			ROW_VALUE.iTable = child_data_address
			ROW_VALUE.affinity = SQLITE_AFF_INTEGER
			CHILD_COLUMN.iTable = sources.a[0].iCursor
			CHILD_COLUMN.iColumn = -1
		}
		WHERE = db.ExprAnd(WHERE, pParse.Expr(TK_NE, ROW_VALUE, CHILD_COLUMN, ""))
	}

	//	Resolve the references in the WHERE clause.
	sqlite3ResolveExprNames(&NameContext{ SrcList: sources, Parse: pParse }, WHERE)

	//	Create VDBE to loop through the entries in pSrc that match the WHERE clause. If the constraint is not deferred, throw an exception for each row found. Otherwise, for deferred constraints, increment the deferred constraint counter by nIncr for each row selected.
	WInfo := pParse.WhereBegin(sources, WHERE, nil, nil, 0)
	if constraint_increment > 0 && key.isDeferred == 0 {
		pParse.Toplevel().mayAbort = true
	}
	v.AddOp2(OP_FkCounter, key.isDeferred, constraint_increment)
	if WInfo != nil {
		sqlite3WhereEnd(WInfo)
	}

	//	Clean up the WHERE clause constructed above.
	db.ExprDelete(WHERE)
	if FK_IF_ZERO != 0 {
		v.JumpHere(FK_IF_ZERO)
	}
}

//	This function returns a pointer to the head of a linked list of FK constraints for which table pTab is the parent table. For example, given the following schema:
//		CREATE TABLE t1(a PRIMARY KEY)
//		CREATE TABLE t2(b REFERENCES t1(a)
//	Calling this function with table "t1" as an argument returns a pointer to the ForeignKey structure representing the foreign key constraint on table "t2". Calling this function with "t2" as the argument would return a NULL pointer (as there are no FK constraints for which t2 is the parent table).
func (t *Table) FkReferences() *ForeignKey {
	return t.Schema.ForeignKeys[t.Name]
}

/*
** The second argument is a Trigger structure allocated by the 
** fkActionTrigger() routine. This function deletes the Trigger structure
** and all of its sub-components.
**
** The Trigger structure or any of its sub-components may be allocated from
** the lookaside buffer belonging to database handle dbMem.
*/
static void fkTriggerDelete(sqlite3 *dbMem, Trigger *p){
  if( p ){
    TriggerStep *pStep = p.Steps;
    dbMem.ExprDelete(pStep.Where)
    dbMem.ExprListDelete(pStep.ExprList);
    sqlite3SelectDelete(dbMem, pStep.Select);
    dbMem.ExprDelete(p.pWhen)
    p = nil
  }
}

/*
** This function is called to generate code that runs when table pTab is
** being dropped from the database. The SrcList passed as the second argument
** to this function contains a single entry guaranteed to resolve to
** table pTab.
**
** Normally, no code is required. However, if either
**
**   (a) The table is the parent table of a FK constraint, or
**   (b) The table is the child table of a deferred FK constraint and it is
**       determined at runtime that there are outstanding deferred FK 
**       constraint violations in the database,
**
** then the equivalent of "DELETE FROM <tbl>" is executed before dropping
** the table from the database. Triggers are disabled while running this
** DELETE, but foreign key actions are not.
*/
 void sqlite3FkDropTable(Parse *pParse, SrcList *pName, Table *pTab){
  sqlite3 *db = pParse.db;
  if( (db.flags&SQLITE_ForeignKeys) && !pTab.IsVirtual() && !pTab.Select ){
    int iSkip = 0;
    Vdbe *v = pParse.GetVdbe()

    assert( v );                  /* VDBE has already been allocated */
    if pTab.FkReferences() == nil {
		//	Search for a deferred foreign key constraint for which this table is the child table. If one cannot be found, return without generating any VDBE code. If one can be found, then jump over the entire DELETE if there are no outstanding deferred constraints when this statement is run.
      ForeignKey *p;
      for(p=pTab.ForeignKey; p; p=p.NextFrom){
        if( p.isDeferred ) break;
      }
      if( !p ) return;
      iSkip = v.MakeLabel()
      v.AddOp2(OP_FkIfZero, 1, iSkip);
    }

    pParse.disableTriggers = 1;
    sqlite3DeleteFrom(pParse, pName.Dup(0), 0)
    pParse.disableTriggers = 0;

    /* If the DELETE has generated immediate foreign key constraint 
    ** violations, halt the VDBE and return an error at this point, before
    ** any modifications to the schema are made. This is because statement
    ** transactions are not able to rollback schema changes.  */
    v.AddOp2(OP_FkIfZero, 0, v.CurrentAddr() + 2)
    sqlite3HaltConstraint(
        pParse, OE_Abort, "foreign key constraint failed", P4_STATIC
    );

    if( iSkip ){
      v.ResolveLabel(iSkip)
    }
  }
}

//	This function is called when inserting, deleting or updating a row of table pTab to generate VDBE code to perform foreign key constraint processing for the operation.
//	For a DELETE operation, parameter oldRow is passed the index of the first register in an array of (pTab.nCol+1) registers containing the rowid of the row being deleted, followed by each of the column values of the row being deleted, from left to right. Parameter newRow is passed zero in this case.
//	For an INSERT operation, oldRow is passed zero and newRow is passed the first register of an array of (pTab.nCol+1) registers containing the new row data.
//	For an UPDATE operation, this function is called twice. Once before the original record is deleted from the table using the calling convention described for DELETE. Then again after the original record is deleted but before the new record is inserted using the INSERT convention. 
func (p *Parse) FkCheck(table *Table, oldRow, newRow int) {
	//	Exactly one of oldRow and newRow should be non-zero.
	assert( (oldRow == 0) != (newRow == 0) )

	db := p.db
	if db.flags & SQLITE_ForeignKeys != 0 {
		iDb := db.SchemaToIndex(table.Schema)

		//	Loop through all the foreign key constraints for which pTab is the child table (the table that the foreign key definition is part of).
		for key := table.ForeignKey; key != nil; key = key.NextFrom {
			int iCol

			//	Find the parent table of this foreign key. Also find a unique index on the parent key columns in the parent table. If either of these schema items cannot be located, set an error in pParse and return early.
			if p.disableTriggers {
				pTo = db.FindTable(key.zTo, db.Databases[iDb].Name)
			} else {
				pTo = p.LocateTable(key.zTo, db.Databases[iDb].Name, false)
			}
			index, columns, rc := p.LocateFkeyIndex(pTo, key)
			if pTo == nil || rc != SQLITE_OK {
				assert( !p.disableTriggers || (oldRow != 0 && newRow == 0) )
				if !p.disableTriggers || db.mallocFailed {
					return
				}
				if pTo == 0 {
					//	If isIgnoreErrors is true, then a table is being dropped. In this case SQLite runs a "DELETE FROM xxx" on the table being dropped before actually dropping it in order to check FK constraints. If the parent table of an FK constraint on the current table is missing, behave as if it is empty. i.e. decrement the relevant FK counter for each row of the current table with non-NULL keys.
					v := p.GetVdbe()
					iJump := v.CurrentAddr() + key.nCol + 1
					for i := 0; i < key.nCol; i++ {
						v.AddOp2(OP_IsNull, (key.Columns[i].iFrom + oldRow + 1), iJump)
					}
					v.AddOp2(OP_FkCounter, key.isDeferred, -1)
				}
				continue
			}
			assert( key.nCol == 1 || (columns != nil && index != nil) )

			if columns == nil {
				iCol = key.Columns[0].iFrom
	      		columns = &iCol
			}

			sqlite_ignore := false
			for i := 0; i < key.nCol; i++ {
				if columns[i] == table.iPKey {
					columns[i] = -1
				}
				//	Request permission to read the parent key columns. If the authorization callback returns SQLITE_IGNORE, behave as if any values read from the parent table are NULL.
				if db.xAuth != nil {
					column := pTo.Columns[index ? index.Columns[i] : pTo.iPKey].Name
					rcauth := p.AuthReadColumn(pTo.Name, column, iDb)
					sqlite_ignore = (rcauth == SQLITE_IGNORE)
				}
			}

			//	Take a shared-cache advisory read-lock on the parent table. Allocate a cursor to use to search the unique index on the parent key columns in the parent table.
			p.TableLock(iDb, pTo.tnum, pTo.Name, false)
			p.nTab++

			if oldRow != 0 {
				//	A row is being removed from the child table. Search for the parent. If the parent does not exist, removing the child row resolves an outstanding foreign key constraint violation.
				p.fkLookupParent(iDb, pTo, index, key, columns, oldRow, -1, sqlite_ignore)
			}
			if newRow != 0 {
				//	A row is being added to the child table. If a parent row cannot be found, adding the child row has violated the FK constraint.
				p.fkLookupParent(iDb, pTo, index, key, columns, newRow, +1, sqlite_ignore)
			}
		}

		//	Loop through all the foreign key constraints that refer to this table
		for key := table.FkReferences(); key != nil; key = key.NextTo) {
			if !key.isDeferred && !p.pToplevel && !p.isMultiWrite {
				assert( oldRow == 0 && newRow != 0 )
				//	Inserting a single row into a parent table cannot cause an immediate foreign key violation. So do nothing in this case.
				continue
			}

			index, columns, rc := p.LocateFkeyIndex(table, key)
			if rc != SQLITE_OK {
				if !isIgnoreErrors || db.mallocFailed {
					return
				}
				continue
	    	}
	    	assert( columns != nil || key.nCol == 1 )

			//	Create a SrcList structure containing a single table (the table the foreign key that refers to this table is attached to). This is required for the sqlite3WhereXXX() interface.
			if pSrc := db.SrcListAppend(nil, "", ""); pSrc != nil {
				struct SrcList_item *pItem = pSrc.a
				pItem.pTab = key.pFrom
				pItem.Name = key.pFrom.Name
				pItem.pTab.nRef++
				pItem.iCursor = p.nTab++
  
				if newRow != 0 {
					p.fkScanChildren(pSrc, table, index, key, columns, newRow, -1)
				}
				if oldRow != 0 {
					//	If there is a RESTRICT action configured for the current operation on the parent table of this FK, then throw an exception immediately if the FK constraint is violated, even if this is a deferred trigger. That's what RESTRICT means. To defer checking the constraint, the FK should specify NO ACTION (represented using OE_None). NO ACTION is the default.
					p.fkScanChildren(pSrc, table, index, key, columns, oldRow, 1)
				}
				pItem.Name = ""
				sqlite3SrcListDelete(db, pSrc)
			}
		}
	}
	return
}

#define COLUMN_MASK(x) (((x)>31) ? 0xffffffff : ((uint32)1<<(x)))

//	This function is called before generating code to update or delete a row contained in table pTab.
func (p *Parse) FkOldmask(t *Table) (mask uint32) {
	if p.db.flags & SQLITE_ForeignKeys {
		for f := t.ForeignKey; f != nil; f = f.NextFrom){
			for i := 0; i < f.nCol; i++ {
				mask |= COLUMN_MASK(f.Columns[i].iFrom)
			}
		}
		for f := pTab.FkReferences(); f != nil; f = f.NextTo) {
			if index, _, _  := p.LocateFkeyIndex(t, f); index != nil {
				for _, column := range index.Columns {
					mask |= COLUMN_MASK(column)
				}
			}
		}
	}
	return mask
}


//	This function is called before generating code to update or delete a row contained in table pTab. If the operation is a DELETE, then parameter aChange is passed a NULL value. For an UPDATE, aChange points to an array of size N, where N is the number of columns in table pTab. If the i'th column is not modified by the UPDATE, then the corresponding entry in the aChange[] array is set to -1. If the column is modified, the value is 0 or greater. Parameter chngRowid is set to true if the UPDATE statement modifies the rowid fields of the table.
//	If any foreign key processing will be required, this function returns true. If there is no foreign key related processing, this function returns false.
int sqlite3FkRequired(
  Parse *pParse,                  /* Parse context */
  Table *pTab,                    /* Table being modified */
  int *aChange,                   /* Non-NULL for UPDATE operations */
  int chngRowid                   /* True for UPDATE that affects rowid */
){
	if pParse.db.flags&SQLITE_ForeignKeys {
		if !aChange {
			//	A DELETE operation. Foreign key processing is required if the table in question is either the child or parent table for any foreign key constraint.
			return pTab.FkReferences() || pTab.ForeignKey
		} else {
			//	This is an UPDATE. Foreign key processing is only required if the operation modifies one or more child or parent key columns.

			//	Check if any child key columns are being modified.
	  		for f := pTab.ForeignKey; f != nil; f = f.NextFrom {
				for i := 0; i < f.nCol; i++ {
					iChildKey := f.Columns[i].iFrom
					if aChange[iChildKey] >= 0 {
						return 1
					}
					if iChildKey == pTab.iPKey && chngRowid {
						return 1
					}
				}
			}

			//	Check if any parent key columns are being modified.
			for f := pTab.FkReferences(); f != nil; f = f.NextTo {
				for i := 0; i < f.nCol; i++ {
					zKey := f.Columns[i].zCol
					for iKey := 0; iKey < pTab.nCol; iKey++ {
						pCol = &pTab.Columns[iKey]
						if (zKey != "" ? CaseInsensitiveMatch(pCol.Name, zKey) : pCol.isPrimKey) {
							if aChange[iKey] >= 0 {
								return 1
							}
							if iKey == pTab.iPKey && chngRowid {
								return 1
							}
						}
					}
				}
			}
		}
	}
	return 0
}

//	This function is called when an UPDATE or DELETE operation is being compiled on table pTab, which is the parent table of foreign-key f. If the current operation is an UPDATE, then the pChanges parameter is passed a pointer to the list of columns being modified. If it is a DELETE, pChanges is passed a NULL pointer.
//	It returns a pointer to a Trigger structure containing a trigger equivalent to the ON UPDATE or ON DELETE action specified by f. If the action is "NO ACTION" or "RESTRICT", then a NULL pointer is returned (these actions require no special handling by the triggers sub-system, code for them is created by fkScanChildren()).
//	For example, if ForeignKey is the foreign key and pTab is table "p" in the following schema:
//		CREATE TABLE p(pk PRIMARY KEY);
//		CREATE TABLE c(ck REFERENCES p ON DELETE CASCADE);
//	then the returned trigger structure is equivalent to:
//		CREATE TRIGGER ... DELETE ON p BEGIN
//			DELETE FROM c WHERE ck = old.pk;
//		END;
//	The returned pointer is cached as part of the foreign key object. It is eventually freed along with the rest of the foreign key object by sqlite3FkDelete().

func (p *Parse) func fkActionTrigger(table *Table, key *ForeignKey, changes *ExprList) (trigger *Trigger)
	db := p.db
	action := key.aAction[iAction]
	trigger = key.apTrigger[iAction]		//	One of OE_None, OE_Cascade etc.
	iAction := changes != nil			//	true for UPDATE, false for DELETE

	if action != OE_None && trigger != nil {
		index, columns, rc := p.LocateFkeyIndex(table, key)
		if rc != SQLITE_OK {
			return
		}
		assert( columns != nil || key.nCol == 1 )

		ToCol, FromCol				string
		FromColIndex				int
		NEW, EQ, WHERE, WHEN		*Expr
		ON_CASCADE					*ExprList			//	Changes list if ON UPDATE CASCADE
		for i := 0; i < key.nCol; i++ {
			if columns == nil {
				FromColIndex = key.Columns[0].iFrom
			} else {
				FromColIndex = columns[i]
			}
			assert( FromColIndex >= 0 )

			if index != nil {
				ToCol = table.Columns[index.Columns[i]].Name
			} else {
				ToCol = "oid"
			}
			FromCol := key.pFrom.Columns[FromColIndex].Name

			//	Create the expression "OLD.ToCol = FromCol". It is important that the "OLD.ToCol" term is on the LHS of the = operator, so that the affinity and collation sequence associated with the parent table are used for the comparison.
			EQ := p.Expr(TK_EQ,
							p.Expr(TK_DOT,
									p.Expr(TK_ID, nil, nil, "old"),
									p.Expr(TK_ID, nil, nil, ToCol), ""),
							p.Expr(TK_ID, nil, nil, FromCol),
							"")
			WHERE = db.ExprAnd(WHERE, EQ)

			//	For ON UPDATE, construct the next term of the WHEN clause. The final WHEN clause will be like this:
			//		WHEN NOT(old.col1 IS new.col1 AND ... AND old.colN IS new.colN)
			if changes != nil {
				EQ = p.Expr(TK_IS,
						p.Expr(TK_DOT,
								p.Expr(TK_ID, nil, nil, "old"),
								p.Expr(TK_ID, nil, nil, ToCol), ""),
						p.Expr(TK_DOT,
								p.Expr(TK_ID, nil, nil, "new"),
								p.Expr(TK_ID, nil, nil, ToCol), ""),
						"")
				WHEN = db.ExprAnd(WHEN, EQ)
			}

			if action != OE_Restrict && (action != OE_Cascade || changes != nil) {
				switch action {
				case OE_Cascade:
					NEW = p.Expr(TK_DOT,
									p.Expr(TK_ID, nil, nil, "new"),
									p.Expr(TK_ID, nil, nil, ToCol), "")
				case OE_SetDflt:
					if pDflt := key.pFrom.Columns[FromColIndex].pDflt; pDflt != nil {
						NEW = pDflt.Dup()
					} else {
						NEW = p.Expr(TK_NULL, nil, nil, "")
					}
				default:
					NEW = p.Expr(TK_NULL, nil, nil, "")
				}
				pList = append(ON_CASCADE, NEW)
				ON_CASCADE.SetName(FromCol, false)
			}
		}

		SELECT		*Select					//	If RESTRICT, "SELECT RAISE(...)"
		FromTable := key.pFrom.zName
		if action == OE_Restrict {
			RAISE := db.Expr(TK_RAISE, "foreign key constraint failed")
			if RAISE != nil {
				RAISE.affinity = OE_Abort
			}
			SELECT = sqlite3SelectNew(p, NewExprList(RAISE), db.SrcListAppend(nil, FromTable, ""), WHERE, 0, 0, 0, 0, 0, 0)
			WHERE = nil
		}

		//	Disable lookaside memory allocation
		enableLookaside = db.lookaside.bEnabled
		db.lookaside.bEnabled = false

		trigger = new(Trigger)
		trigger.Steps = new(TriggerStep)
		step := trigger.Steps
		step.Target = FromTable
		step.Where = WHERE.Dup()
		step.ExprList = ON_CASCADE.Dup()
		step.Select = SELECT.Dup()
		if WHEN != nil {
			WHEN = p.Expr(TK_NOT, WHEN, nil, "")
			trigger.pWhen = WHEN.Dup()
		}

		//	Re-enable the lookaside buffer, if it was disabled earlier.
		db.lookaside.bEnabled = enableLookaside

		db.ExprDelete(WHERE)
		db.ExprDelete(WHEN)
		db.ExprListDelete(ON_CASCADE)
		sqlite3SelectDelete(db, SELECT)
		if db.mallocFailed {
			fkTriggerDelete(db, trigger)
			return 0
		}
		assert( step != nil )

		switch( action ){
		case OE_Restrict:
			step.op = TK_SELECT
		case OE_Cascade: 
        	if changes == nil { 
				step.op = TK_DELETE
			} else {
				fallthrough
			}
		default:
			step.op = TK_UPDATE
		}
		step.Trigger = trigger
		trigger.Schema = table.Schema
		trigger.TableSchema = table.Schema
		f.apTrigger[iAction] = trigger
		if changes == nil {
			trigger.op = TK_DELETE
		} else {
			trigger.op = TK_UPDATE
		}
	}
	return
}

//	This function is called when deleting or updating a row to implement any required CASCADE, SET NULL or SET DEFAULT actions.
void sqlite3FkActions(
  Parse *pParse,                  /* Parse context */
  Table *pTab,                    /* Table being updated or deleted from */
  ExprList *pChanges,             /* Change-list for UPDATE, NULL for DELETE */
  int oldRow                      /* Address of array containing old row */
){
	//	If foreign-key support is enabled, iterate through all FKs that refer to table pTab. If there is an action associated with the FK for this operation (either update or delete), invoke the associated trigger sub-program.
	if pParse.db.flags & SQLITE_ForeignKeys {
		for f := pTab.FkReferences(); f != nil; f = f.NextTo {
			if pAction := fkActionTrigger(pParse, pTab, f, pChanges); pAction != nil {
				sqlite3CodeRowTriggerDirect(pParse, pAction, pTab, oldRow, OE_Abort, 0)
			}
		}
	}
}

//	Free all memory associated with foreign key definitions attached to table pTab. Remove the deleted foreign keys from the Schema.ForeignKeys hash table.
 void sqlite3FkDelete(sqlite3 *db, Table *pTab){
  next		*ForeignKey

  assert( db==0 )
  for f := pTab.ForeignKey; f != nil; f = next {

    //	Remove the FK from the ForeignKeys hash table.
    if !db || db.pnBytesFreed == 0 {
      if f.pPrevTo {
        f.pPrevTo.NextTo = f.NextTo
      } else {
        void *p = (void *)f.NextTo
        const char *z = (p ? ForeignKey.NextTo.zTo : ForeignKey.zTo)
        pTab.Schema.ForeignKeys[z] = p
      }
      if f.NextTo {
        f.NextTo.pPrevTo = f.pPrevTo;
      }
    }

    //	Delete any triggers created to implement actions for this FK.
    fkTriggerDelete(db, f.apTrigger[0])
    fkTriggerDelete(db, f.apTrigger[1])
    next = f.NextFrom
    f = nil
  }
}