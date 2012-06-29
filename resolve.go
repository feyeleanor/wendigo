/* This file contains routines used for walking the parser tree and
** resolve all identifiers by associating them with a particular
** table and column.
*/
/* #include <stdlib.h> */
/* #include <string.h> */

//	Turn the pExpr expression into an alias for the iCol-th column of the result set in pEList.
//	If the result set column is a simple column reference, then this routine makes an exact copy. But for any other kind of expression, this routine make a copy of the result set column as the argument to the TK_AS operator. The TK_AS operator causes the expression to be evaluated just once and then reused for each alias.
//	The reason for suppressing the TK_AS term when the expression is a simple column reference is so that the column reference will be recognized as usable by indices within the WHERE clause processing logic. 
//	Hack:  The TK_AS operator is inhibited if zType[0]=='G'. This means that in a GROUP BY clause, the expression is evaluated twice.  Hence:
//			SELECT random()%5 AS x, count(*) FROM tab GROUP BY x
//	Is equivalent to:
//			SELECT random()%5 AS x, count(*) FROM tab GROUP BY random()%5
//	The result of random()%5 in the GROUP BY clause is probably different from the result in the result-set. We might fix this someday. Or then again, we might not...
func (pParse *Parse) resolveAlias(pEList *ExprList, iCol int, pExpr *Expr, zType string) (pDup *Expr) {
	assert( iCol >= 0 && iCol < pEList.Len() )
	pOrig := pEList.Items[iCol].Expr
	assert( pOrig != nil )
	assert( pOrig.flags & EP_Resolved )
	db := pParse.db
	switch {
	case pOrig.op != TK_COLUMN && zType[0] != 'G':
		pDup = pOrig.Dup()
		pDup = pParse.Expr(TK_AS, pDup, nil, "")
		if pDup == nil {
			return
		}
		if pEList.a[iCol].iAlias == 0 {
			pEList.a[iCol].iAlias = uint16(++pParse.nAlias)
		}
		pDup.iTable = pEList.a[iCol].iAlias
	case pOrig.HasProperty(EP_IntValue) || pOrig.Token == "":
		pDup = pOrig.Dup()
		if pDup == nil {
			return
		}
	default:
		Token := pOrig.Token
		assert( Token != "" )
		pOrig.Token = ""
		pDup = pOrig.Dup()
		pOrig.Token = Token
		if pDup == nil {
			return
		}
		pDup.Token = sqlite3DbStrDup(db, Token)
	}
	if pExpr.flags & EP_ExpCollate {
		pDup.pColl = pExpr.pColl
		pDup.flags |= EP_ExpCollate;
	}
	db.ExprDelete(pExpr)
	return
}


/*
** Return TRUE if the name zCol occurs anywhere in the USING clause.
**
** Return FALSE if the USING clause is NULL or if it does not contain
** zCol.
*/
static int nameInUsingClause(IdList *pUsing, const char *zCol){
  if( pUsing ){
    int k;
    for(k=0; k<pUsing.nId; k++){
      if CaseInsensitiveMatch(pUsing.a[k].Name, zCol) {
		  return 1
		}
    }
  }
  return 0;
}


/*
** Given the name of a column of the form X.Y.Z or Y.Z or just Z, look up
** that name in the set of source tables in pSrcList and make the pExpr 
** expression node refer back to that source column.  The following changes
** are made to pExpr:
**
**    pExpr.iDb           Set the index in db.Databases[] of the database X
**                         (even if X is implied).
**    pExpr.iTable        Set to the cursor number for the table obtained
**                         from pSrcList.
**    pExpr.pTab          Points to the Table structure of X.Y (even if
**                         X and/or Y are implied.)
**    pExpr.iColumn       Set to the column number within the table.
**    pExpr.op            Set to TK_COLUMN.
**    pExpr.pLeft         Any expression this points to is deleted
**    pExpr.pRight        Any expression this points to is deleted.
**
** The zDb variable is the name of the database (the "X").  This value may be
** NULL meaning that name is of the form Y.Z or Z.  Any available database
** can be used.  The zTable variable is the name of the table (the "Y").  This
** value can be NULL if zDb is also NULL.  If zTable is NULL it
** means that the form of the name is Z and that columns from any table
** can be used.
**
** If the name cannot be resolved unambiguously, leave an error message
** in pParse and return WRC_Abort.  Return WRC_Prune on success.
*/
static int lookupName(
  Parse *pParse,       /* The parsing context */
  const char *zDb,     /* Name of the database containing table, or NULL */
  const char *zTab,    /* Name of table containing column, or NULL */
  const char *zCol,    /* Name of the column. */
  NameContext *pNC,    /* The name context used to resolve the name */
  Expr *pExpr          /* Make this EXPR node point to the selected column */
){
  int i, j;            /* Loop counters */
  int cnt = 0;                      /* Number of matching column names */
  int cntTab = 0;                   /* Number of matching table names */
  sqlite3 *db = pParse.db;         /* The database connection */
  struct SrcList_item *pItem;       /* Use for looping over pSrcList items */
  struct SrcList_item *pMatch = 0;  /* The matching pSrcList item */
  pTopNC := pNC						//	First namecontext in the list
  schema	*Schema						//	Schema of the expression
  int isTrigger = 0;

  assert( pNC != nil )
  assert( zCol );    /* The Z in X.Y.Z cannot be NULL */

  /* Initialize the node to no-match */
  pExpr.iTable = -1;
  pExpr.pTab = 0;

  /* Start at the inner-most context and move outward until a match is found */
  while( pNC && cnt==0 ){
    ExprList *pEList;
    pSrcList := pNC.SrcList

    if( pSrcList ){
      for(i=0, pItem=pSrcList.a; i<pSrcList.nSrc; i++, pItem++){
        Table *pTab;
        int iDb;
        Column *pCol;
  
        pTab = pItem.pTab;
        assert( pTab!=0 && pTab.Name!=0 );
        iDb = db.SchemaToIndex(pTab.Schema)
        assert( pTab.nCol>0 );
        if( zTab ){
          if( pItem.zAlias ){
            if !CaseInsensitiveMatch(pItem.zAlias, zTab) {
				continue
			}
          }else{
            if !CaseInsensitiveMatch(pTab.Name, zTab) || !CaseInsensitiveMatch(db.Databases[iDb].Name, zDb) {
              continue;
            }
          }
        }
        if( 0==(cntTab++) ){
          pExpr.iTable = pItem.iCursor;
          pExpr.pTab = pTab;
          schema = pTab.Schema;
          pMatch = pItem;
        }
        for(j=0, pCol=pTab.Columns; j<pTab.nCol; j++, pCol++){
          if CaseInsensitiveMatch(pCol.Name, zCol) {
            /* If there has been exactly one prior match and this match
            ** is for the right-hand table of a NATURAL JOIN or is in a 
            ** USING clause, then skip this match.
            */
            if( cnt==1 ){
              if( pItem.jointype & JT_NATURAL ) continue;
              if( nameInUsingClause(pItem.pUsing, zCol) ) continue;
            }
            cnt++;
            pExpr.iTable = pItem.iCursor;
            pExpr.pTab = pTab;
            pMatch = pItem;
            schema = pTab.Schema;
            /* Substitute the rowid (column -1) for the INTEGER PRIMARY KEY */
            pExpr.iColumn = j==pTab.iPKey ? -1 : (int16)j;
            break;
          }
        }
      }
    }

    /* If we have not already resolved the name, then maybe 
    ** it is a new.* or old.* trigger argument reference
    */
    if( zDb==0 && zTab!=0 && cnt==0 && pParse.pTriggerTab!=0 ){
      int op = pParse.eTriggerOp;
      Table *pTab = 0;
      assert( op==TK_DELETE || op==TK_UPDATE || op==TK_INSERT );
      if op!=TK_DELETE && CaseInsensitiveMatch("new",zTab) {
        pExpr.iTable = 1;
        pTab = pParse.pTriggerTab;
      }else if op!=TK_INSERT && CaseInsensitiveMatch("old",zTab) {
        pExpr.iTable = 0;
        pTab = pParse.pTriggerTab;
      }

      if( pTab ){ 
        int iCol;
        schema = pTab.Schema;
        cntTab++;
        for(iCol=0; iCol<pTab.nCol; iCol++){
          Column *pCol = &pTab.Columns[iCol];
          if CaseInsensitiveMatch(pCol.Name, zCol) {
            if( iCol==pTab.iPKey ){
              iCol = -1;
            }
            break;
          }
        }
        if( iCol>=pTab.nCol && sqlite3IsRowid(zCol) ){
          iCol = -1;        /* IMP: R-44911-55124 */
        }
        if( iCol<pTab.nCol ){
          cnt++;
          if( iCol<0 ){
            pExpr.affinity = SQLITE_AFF_INTEGER;
          }else if( pExpr.iTable==0 ){
            pParse.oldmask |= (iCol>=32 ? 0xffffffff : (((uint32)1)<<iCol));
          }else{
            pParse.newmask |= (iCol>=32 ? 0xffffffff : (((uint32)1)<<iCol));
          }
          pExpr.iColumn = (int16)iCol;
          pExpr.pTab = pTab;
          isTrigger = 1;
        }
      }
    }

    /*
    ** Perhaps the name is a reference to the ROWID
    */
    if( cnt==0 && cntTab==1 && sqlite3IsRowid(zCol) ){
      cnt = 1;
      pExpr.iColumn = -1;     /* IMP: R-44911-55124 */
      pExpr.affinity = SQLITE_AFF_INTEGER;
    }

	//	If the input is of the form Z (not Y.Z or X.Y.Z) then the name Z might refer to an result-set alias. This happens, for example, when we are resolving names in the WHERE clause of the following command:
	//			SELECT a+b AS x FROM table WHERE x<10;
	//	In cases like this, replace pExpr with a copy of the expression that forms the result set entry ("a+b" in the example) and return immediately. Note that the expression in the result set should have already been resolved by the time the WHERE clause is resolved.
	if cnt == 0 {
		if pEList = pNC.pEList; pEList != nil && zTab == "" {
			for _, item := range pEList.Items {
				zAs = item.Name
				if CaseInsensitiveMatch(zAs, zCol) {
					assert( pExpr.pLeft == nil && pExpr.pRight == nil )
					assert( pExpr.pList == nil )
					assert( pExpr.x.Select == nil )
					pOrig := item.Expr
					if (pNC.Flags & NC_AllowAgg) == 0 && pOrig.HasProperty(EP_Agg) {
						pParse.SetErrorMsg("misuse of aliased aggregate %v", zAs)
						return WRC_Abort
					}
					pExpr = pParse.resolveAlias(pEList, j, pExpr, "")
					cnt = 1
					pMatch = 0
					assert( zTab == "" && zDb == "" )
					goto lookupname_end;
				}
			}
		} 
	}

	//	Advance to the next name context. The loop will exit when either we have a match (cnt>0) or when we run out of name contexts.
	if cnt == 0 {
		pNC = pNC.Next
	}
  }

  /*
  ** If X and Y are NULL (in other words if only the column name Z is
  ** supplied) and the value of Z is enclosed in double-quotes, then
  ** Z is a string literal if it doesn't match any column names.  In that
  ** case, we need to return right away and not make any changes to
  ** pExpr.
  **
  ** Because no reference was made to outer contexts, the pNC.nRef
  ** fields are not changed in any context.
  */
  if( cnt==0 && zTab==0 && pExpr.HasProperty(EP_DblQuoted) ){
    pExpr.op = TK_STRING;
    pExpr.pTab = 0;
    return WRC_Prune;
  }

  /*
  ** cnt==0 means there was not match.  cnt>1 means there were two or
  ** more matches.  Either way, we have an error.
  */
  if( cnt!=1 ){
    const char *zErr;
    zErr = cnt==0 ? "no such column" : "ambiguous column name";
    if( zDb ){
      pParse.SetErrorMsg("%v: %v.%v.%v", zErr, zDb, zTab, zCol);
    }else if( zTab ){
      pParse.SetErrorMsg("%v: %v.%v", zErr, zTab, zCol);
    }else{
      pParse.SetErrorMsg("%v: %v", zErr, zCol);
    }
    pParse.checkSchema = 1;
    pTopNC.Errors++
  }

  /* If a column from a table in pSrcList is referenced, then record
  ** this fact in the pSrcList.a[].colUsed bitmask.  Column 0 causes
  ** bit 0 to be set.  Column 1 sets bit 1.  And so forth.  If the
  ** column number is greater than the number of bits in the bitmask
  ** then set the high-order bit of the bitmask.
  */
  if( pExpr.iColumn>=0 && pMatch!=0 ){
    int n = pExpr.iColumn;
    if( n>=BMS ){
      n = BMS-1;
    }
    assert( pMatch.iCursor==pExpr.iTable );
    pMatch.colUsed |= ((Bitmask)1)<<n;
  }

  /* Clean up and return
  */
  db.ExprDelete(pExpr.pLeft)
  pExpr.pLeft = 0;
  db.ExprDelete(pExpr.pRight)
  pExpr.pRight = 0;
  pExpr.op = (isTrigger ? TK_TRIGGER : TK_COLUMN);
lookupname_end:
  if( cnt==1 ){
    assert( pNC != nil );
    pParse.AuthRead(pExpr, schema , pNC.SrcList)
    //	Increment the nRef value on all name contexts from TopNC up to the point where the name matched.
    for(;;){
      assert( pTopNC != nil )
      pTopNC.References++
      if( pTopNC == pNC ) break;
      pTopNC = pTopNC.Next
    }
    return WRC_Prune;
  } else {
    return WRC_Abort;
  }
}

/*
** Allocate and return a pointer to an expression to load the column iCol
** from datasource iSrc in SrcList pSrc.
*/
 Expr *sqlite3CreateColumnExpr(sqlite3 *db, SrcList *pSrc, int iSrc, int iCol){
  Expr *p = sqlite3ExprAlloc(db, TK_COLUMN, 0, 0);
  if( p ){
    struct SrcList_item *pItem = &pSrc.a[iSrc];
    p.pTab = pItem.pTab;
    p.iTable = pItem.iCursor;
    if( p.pTab.iPKey==iCol ){
      p.iColumn = -1;
    }else{
      p.iColumn = (ynVar)iCol;
      pItem.colUsed |= ((Bitmask)1)<<(iCol>=BMS ? BMS-1 : iCol);
    }
    p.SetProperty(EP_Resolved);
  }
  return p;
}

//	This routine is callback for Expr().
//	Resolve symbolic names into TK_COLUMN operators for the current node in the expression tree. Return 0 to continue the search down the tree or 2 to abort the tree walk.
//	This routine also does error checking and name resolution for function names.  The operator for aggregate functions is changed to TK_AGG_FUNCTION.
static int resolveExprStep(Walker *pWalker, Expr *pExpr){
  NameContext *pNC;
  Parse *pParse;

  pNC = pWalker.u.pNC;
  assert( pNC!=0 );
  pParse = pNC.Parse;
  assert( pParse==pWalker.pParse );

  if( pExpr.HasAnyProperty(EP_Resolved) ) return WRC_Prune;
  pExpr.SetProperty(EP_Resolved);
  switch( pExpr.op ){

#if defined(SQLITE_ENABLE_UPDATE_DELETE_LIMIT)
    /* The special operator TK_ROW means use the rowid for the first
    ** column in the FROM clause.  This is used by the LIMIT and ORDER BY
    ** clause processing on UPDATE and DELETE statements.
    */
    case TK_ROW: {
      pSrcList := pNC.SrcList;
      struct SrcList_item *pItem;
      assert( pSrcList && pSrcList.nSrc==1 );
      pItem = pSrcList.a; 
      pExpr.op = TK_COLUMN;
      pExpr.pTab = pItem.pTab;
      pExpr.iTable = pItem.iCursor;
      pExpr.iColumn = -1;
      pExpr.affinity = SQLITE_AFF_INTEGER;
      break;
    }
#endif /* defined(SQLITE_ENABLE_UPDATE_DELETE_LIMIT) && !defined(SQLITE_OMIT_SUBQUERY) */

    /* A lone identifier is the name of a column.
    */
    case TK_ID: {
      return lookupName(pParse, 0, 0, pExpr.Token, pNC, pExpr);
    }
  
    /* A table name and column name:     ID.ID
    ** Or a database, table and column:  ID.ID.ID
    */
    case TK_DOT: {
      const char *zColumn;
      const char *zTable;
      const char *zDb;
      Expr *pRight;

      /* if( pSrcList==0 ) break; */
      pRight = pExpr.pRight;
      if( pRight.op==TK_ID ){
        zDb = 0;
        zTable = pExpr.pLeft.Token;
        zColumn = pRight.Token;
      }else{
        assert( pRight.op==TK_DOT );
        zDb = pExpr.pLeft.Token;
        zTable = pRight.pLeft.Token;
        zColumn = pRight.pRight.Token;
      }
      return lookupName(pParse, zDb, zTable, zColumn, pNC, pExpr);
    }

    /* Resolve function names
    */
    case TK_CONST_FUNC:
    case TK_FUNCTION: {
      pList := pExpr.pList;    /* The argument list */
      n := pList.Len();    /* Number of arguments */
      int no_such_func = 0;       /* True if no such function exists */
      int wrong_num_args = 0;     /* True if wrong number of arguments */
      int is_agg = 0;             /* True if is an aggregate function */
      int auth;                   /* Authorization to use the function */
      int nId;                    /* Number of characters in function name */
      const char *zId;            /* The function name. */
      FuncDef *pDef;              /* Information about the function */

      enc := pParse.db.Encoding()   /* The database encoding */

      assert( !pExpr.HasProperty(EP_xIsSelect) );
      zId = pExpr.Token;
      nId = sqlite3Strlen30(zId);
      pDef = pParse.db.FindFunction(zId, n, enc, false)
      if( pDef==0 ){
        pDef = pParse.db.FindFunction(zId, -2, enc, false)
        if( pDef==0 ){
          no_such_func = 1;
        }else{
          wrong_num_args = 1;
        }
      }else{
        is_agg = pDef.xFunc==0;
      }
      if( pDef ){
        if auth = pParse.AuthCheck(SQLITE_FUNCTION, 0, pDef.Name, 0); auth != SQLITE_OK {
          if auth == SQLITE_DENY {
            pParse.SetErrorMsg("not authorized to use function: %v", pDef.Name);
            pNC.Errors++
          }
          pExpr.op = TK_NULL;
          return WRC_Prune;
        }
      }
      if( is_agg && (pNC.Flags & NC_AllowAgg)==0 ){
        pParse.SetErrorMsg("misuse of aggregate function %.v%v()", nId,zId);
        pNC.Errors++
        is_agg = 0;
      }else if( no_such_func ){
        pParse.SetErrorMsg("no such function: %.v%v", nId, zId);
        pNC.Errors++
      }else if( wrong_num_args ){
        pParse.SetErrorMsg("wrong number of arguments to function %.v%v()", nId, zId);
        pNC.Errors++
      }
      if( is_agg ){
        pExpr.op = TK_AGG_FUNCTION;
        pNC.Flags |= NC_HasAgg;
      }
      if( is_agg ) pNC.Flags &= ~NC_AllowAgg;
      pWalker.ExprList(pList)
      if( is_agg ) pNC.Flags |= NC_AllowAgg;
      /* FIX ME:  Compute pExpr.affinity based on the expected return
      ** type of the function 
      */
      return WRC_Prune;
    }
    case TK_SELECT:
    case TK_EXISTS:
    case TK_IN: {
      if( pExpr.HasProperty(EP_xIsSelect) ){
        int nRef = pNC.References
        if( (pNC.Flags & NC_IsCheck)!=0 ){
          pParse.SetErrorMsg("subqueries prohibited in CHECK constraints");
        }
        pWalker.Select(pExpr.x.Select)
        assert( pNC.References >= nRef );
        if nRef != pNC.References {
          pExpr.SetProperty(EP_VarSelect);
        }
      }
      break;
    }
    case TK_VARIABLE: {
      if( (pNC.Flags & NC_IsCheck)!=0 ){
        pParse.SetErrorMsg("parameters prohibited in CHECK constraints");
      }
      break;
    }
  }
  return (pParse.nErr || pParse.db.mallocFailed) ? WRC_Abort : WRC_Continue;
}

//	pEList is a list of expressions which are really the result set of the a SELECT statement. pE is a term in an ORDER BY or GROUP BY clause. This routine checks to see if pE is a simple identifier which corresponds to the AS-name of one of the terms of the expression list. If it is, this routine return an integer between 1 and N where N is the number of elements in pEList, corresponding to the matching entry. If there is no match, or if pE is not a simple identifier, then this routine return 0.
//	pEList has been resolved.  pE has not
func (pEList *ExprList) resolveAsName(pE *Expr) int {
	if pE.op == TK_ID {
		for i, item := range pEList.Items {
			zAs = item.Name
			if CaseInsensitiveMatch(zAs, pE.Token) {
				return i + 1
			}
		}
	}
	return 0
}

//	pE is a pointer to an expression which is a single term in the ORDER BY of a compound SELECT. The expression has not been name resolved.
//	At the point this routine is called, we already know that the ORDER BY term is not an integer index into the result set. That case is handled by the calling routine.
//	Attempt to match pE against result set columns in the left-most SELECT statement. Return the index i of the matching column, as an indication to the caller that it should sort by the i-th column. The left-most column is 1.  In other words, the value returned is the same integer value that would be used in the SQL statement to indicate the column.
//	If there is no match, return 0.  Return -1 if an error occurs.
func (pParse *Parse) resolveOrderByTermToExprList(pSelect *Select, pE *Expr) int {
	assert( sqlite3ExprIsInteger(pE, &i) == 0 )
	pEList := pSelect.pEList

	//	Resolve all names in the ORDER BY term expression
	nc := &NameContext{
		Parse:		pParse,
		SrcList:	pSelect.pSrc,
		ExprList:	pEList,
		Flags:		NC_AllowAgg,
	}

	db := pParse.db
	savedSuppErr := db.suppressErr
	db.suppressErr = 1
	rc := sqlite3ResolveExprNames(&nc, pE);
	db.suppressErr = savedSuppErr
	if rc != SQLITE_OK {
		return 0
	}

	//	Try to match the ORDER BY expression against an expression in the result set. Return an 1-based index of the matching result-set entry.
	for i, item := range pEList.Items {
		if item.Expr.Compare(pE) < 2 {
			return i + 1
		}
	}

	//	If no match, return 0.
	return 0
}

/*
** Generate an ORDER BY or GROUP BY term out-of-range error.
*/
static void resolveOutOfRangeError(
  Parse *pParse,         /* The error context into which to write the error */
  const char *zType,     /* "ORDER" or "GROUP" */
  int i,                 /* The index (1-based) of the term out of range */
  int mx                 /* Largest permissible value of i */
){
  pParse.SetErrorMsg("%v %v BY term out of range - should be between 1 and %v", i, zType, mx);
}

//	Analyze the ORDER BY clause in a compound SELECT statement. Modify each term of the ORDER BY clause is a constant integer between 1 and N where N is the number of columns in the compound SELECT.
//	ORDER BY terms that are already an integer between 1 and N are unmodified. ORDER BY terms that are integers outside the range of 1 through N generate an error. ORDER BY terms that are expressions are matched against result set expressions of compound SELECT beginning with the left-most SELECT and working toward the right. At the first match, the ORDER BY expression is transformed into the integer column number.
//	Return the number of errors seen.
func (pParse *Parse) resolveCompoundOrderBy(pSelect *Select) bool {
	if pOrderBy := pSelect.pOrderBy; pOrderBy != nil {
		db = pParse.db
		for _, item := range pOrderBy.Items {
			item.done = false
		}
		pSelect.Next = nil
		for pSelect.pPrior != nil {
			pSelect.pPrior.Next = pSelect
			pSelect = pSelect.pPrior
		}
		for moreToDo := true; pSelect != nil && moreToDo; pSelect = pSelect.Next {
			moreToDo = false
			pEList := pSelect.pEList
			assert( pEList != nil )
			for i, item := range pOrderBy.Items {
				iCol := -1
				if !item.done {
					pE := item.Expr
					if sqlite3ExprIsInteger(pE, &iCol) {
						if iCol <= 0 || iCol > pEList.Len() {
							resolveOutOfRangeError(pParse, "ORDER", i + 1, pEList.Len())
							return false
						}
					} else {
						if iCol = pEList.resolveAsName(pE); iCol == 0 {
							pDup := pE.Dup()
							iCol = pParse.resolveOrderByTermToExprList(pSelect, pDup)
							db.ExprDelete(pDup)
						}
					}
					if iCol > 0 {
						pColl := pE.pColl
						flags := pE.flags & EP_ExpCollate
						db.ExprDelete(pE)
						pE = db.Expr(TK_INTEGER, "")
						item.Expr = pE
						if pE == nil {
							return false
						}
						pE.pColl = pColl
						pE.flags |= EP_IntValue | flags
						pE.Value = iCol
						item.iOrderByCol = uint16(iCol)
						item.done = true
					} else {
						moreToDo = true
					}
				}
			}
		}
		for i, item := range pOrderBy.Items {
			if !item.done {
				pParse.SetErrorMsg("%v ORDER BY term does not match any column in the result set", i + 1)
				return false
			}
		}
	}
	return true
}

//	Check every term in the ORDER BY or GROUP BY clause pOrderBy of the SELECT statement pSelect. If any term is reference to a result set expression (as determined by the ExprList.a.iCol field) then convert that term into a copy of the corresponding result set column.
//	If any errors are detected, add an error message to pParse and return non-zero. Return zero if no errors are seen.
func (pParse *Parse) ResolveOrderGroupBy(pSelect *Select, pOrderBy *ExprList, zType string) bool {
	if pOrderBy != nil && !pParse.db.mallocFailed {
		pEList := pSelect.pEList
		assert( pEList != nil )						//	sqlite3SelectNew() guarantees this
		for i, item := range pOrderBy.Items {
			if item.iOrderByCol != 0 {
				if item.iOrderByCol > pEList.Len() {
					resolveOutOfRangeError(pParse, zType, i + 1, pEList.Len())
					return false
				}
				item.Expr = pParse.resolveAlias(pEList, item.iOrderByCol - 1, item.Expr, zType)
			}
		}
	}
	return true
}

//	pOrderBy is an ORDER BY or GROUP BY clause in SELECT statement pSelect. The Name context of the SELECT statement is pNC. zType is either "ORDER" or "GROUP" depending on which type of clause pOrderBy is.
//	This routine resolves each term of the clause into an expression. If the order-by term is an integer I between 1 and N (where N is the number of columns in the result set of the SELECT) then the expression in the resolution is a copy of the I-th result-set expression. If the order-by term is an identify that corresponds to the AS-name of a result-set expression, then the term resolves to a copy of the result-set expression. Otherwise, the expression is resolved in the usual way - using sqlite3ResolveExprNames().
//	This routine returns the number of errors. If errors occur, then an appropriate error message might be left in pParse. (OOM errors excepted.)
func (pNC *NameContext) resolveOrderGroupBy(pSelect *Select, pOrderBy *ExprList, zType string) bool {
	if pOrderBy != nil {
		nResult := pSelect.pEList.Len()
		pParse := pNC.Parse
		for i, item := range pOrderBy.Items {
			pE := item.Expr
			iCol := pSelect.pEList.resolveAsName(pE)
			if iCol > 0 {
				//	If an AS-name match is found, mark this ORDER BY column as being a copy of the iCol-th result-set column. The subsequent call to ResolveOrderGroupBy() will convert the expression to a copy of the iCol-th result-set expression.
				item.iOrderByCol = uint16(iCol)
			} else {
				if sqlite3ExprIsInteger(pE, &iCol) {
					//	The ORDER BY term is an integer constant. Again, set the column number so that ResolveOrderGroupBy() will convert the order-by term to a copy of the result-set expression
					if iCol < 1 {
						resolveOutOfRangeError(pParse, zType, i + 1, nResult)
						return false
					}
					item.iOrderByCol = uint16(iCol)
				} else {
					//	Otherwise, treat the ORDER BY term as an ordinary expression
					item.iOrderByCol = 0
					if sqlite3ResolveExprNames(pNC, pE) {
						return false
					}
					for j, column := range pSelect.pEList.Items {
						if pE.Compare(column.Expr) == 0 {
							pItem.iOrderByCol = j + 1
						}
					}
				}
			}
		}
		return pParse.ResolveOrderGroupBy(pSelect, pOrderBy, zType)
	}
	return true
}

/*
** Resolve names in the SELECT statement p and all of its descendents.
*/
static int resolveSelectStep(Walker *pWalker, Select *p){
  NameContext *pOuterNC;  /* Context that contains this SELECT */
  NameContext sNC;        /* Name context of this SELECT */
  int isCompound;         /* True if p is a compound select */
  int nCompound;          /* Number of compound terms processed so far */
  Parse *pParse;          /* Parsing context */
  ExprList *pEList;       /* Result set expression list */
  int i;                  /* Loop counter */
  ExprList *pGroupBy;     /* The GROUP BY clause */
  Select *pLeftmost;      /* Left-most of SELECT of a compound */
  sqlite3 *db;            /* Database connection */
  

  assert( p!=0 );
  if( p.selFlags & SF_Resolved ){
    return WRC_Prune;
  }
  pOuterNC = pWalker.u.pNC;
  pParse = pWalker.pParse;
  db = pParse.db;

  /* Normally sqlite3SelectExpand() will be called first and will have
  ** already expanded this SELECT.  However, if this is a subquery within
  ** an expression, sqlite3ResolveExprNames() will be called without a
  ** prior call to sqlite3SelectExpand().  When that happens, let
  ** sqlite3SelectPrep() do all of the processing for this SELECT.
  ** sqlite3SelectPrep() will invoke both sqlite3SelectExpand() and
  ** this routine in the correct order.
  */
  if( (p.selFlags & SF_Expanded)==0 ){
    sqlite3SelectPrep(pParse, p, pOuterNC);
    return (pParse.nErr || db.mallocFailed) ? WRC_Abort : WRC_Prune;
  }

  isCompound = p.pPrior!=0;
  nCompound = 0;
  pLeftmost = p;
  while( p ){
    assert( (p.selFlags & SF_Expanded)!=0 );
    assert( (p.selFlags & SF_Resolved)==0 );
    p.selFlags |= SF_Resolved;

    /* Resolve the expressions in the LIMIT and OFFSET clauses. These
    ** are not allowed to refer to any names, so pass an empty NameContext.
    */
    memset(&sNC, 0, sizeof(sNC));
    sNC.Parse = pParse;
    if( sqlite3ResolveExprNames(&sNC, p.pLimit) ||
        sqlite3ResolveExprNames(&sNC, p.pOffset) ){
      return WRC_Abort;
    }
  
    /* Set up the local name-context to pass to sqlite3ResolveExprNames() to
    ** resolve the result-set expression list.
    */
    sNC.Flags = NC_AllowAgg;
    sNC.SrcList = p.pSrc;
    sNC.Next = pOuterNC;
  
    /* Resolve names in the result set. */
    pEList = p.pEList
    assert( pEList != nil )
	for _, item := range pEList.Items {
      if( sqlite3ResolveExprNames(&sNC, item.Expr) ){
        return WRC_Abort;
      }
    }
  
    /* Recursively resolve names in all subqueries
    */
    for(i=0; i<p.pSrc.nSrc; i++){
      struct SrcList_item *pItem = &p.pSrc.a[i];
      if( pItem.Select ){
        NameContext *pNC;         /* Used to iterate name contexts */
        int nRef = 0;             /* Refcount for pOuterNC and outer contexts */
        const char *zSavedContext = pParse.zAuthContext;

        /* Count the total number of references to pOuterNC and all of its
        ** parent contexts. After resolving references to expressions in
        ** pItem.Select, check if this value has changed. If so, then
        ** SELECT statement pItem.Select must be correlated. Set the
        ** pItem.isCorrelated flag if this is the case. */
        for(pNC=pOuterNC; pNC; pNC=pNC.Next) nRef += pNC.References;

        if( pItem.Name ) pParse.zAuthContext = pItem.Name;
        sqlite3ResolveSelectNames(pParse, pItem.Select, pOuterNC);
        pParse.zAuthContext = zSavedContext;
        if( pParse.nErr || db.mallocFailed ) return WRC_Abort;

        for(pNC=pOuterNC; pNC; pNC=pNC.Next) nRef -= pNC.References;
        assert( pItem.isCorrelated==0 && nRef<=0 );
        pItem.isCorrelated = (nRef!=0);
      }
    }
  
    /* If there are no aggregate functions in the result-set, and no GROUP BY 
    ** expression, do not allow aggregates in any of the other expressions.
    */
    assert( (p.selFlags & SF_Aggregate)==0 );
    pGroupBy = p.pGroupBy;
    if( pGroupBy || (sNC.Flags & NC_HasAgg)!=0 ){
      p.selFlags |= SF_Aggregate;
    }else{
      sNC.Flags &= ~NC_AllowAgg;
    }
  
    /* If a HAVING clause is present, then there must be a GROUP BY clause.
    */
    if( p.pHaving && !pGroupBy ){
      pParse.SetErrorMsg("a GROUP BY clause is required before HAVING");
      return WRC_Abort;
    }
  
    /* Add the expression list to the name-context before parsing the
    ** other expressions in the SELECT statement. This is so that
    ** expressions in the WHERE clause (etc.) can refer to expressions by
    ** aliases in the result set.
    **
    ** Minor point: If this is the case, then the expression will be
    ** re-evaluated for each reference to it.
    */
    sNC.pEList = p.pEList;
    if( sqlite3ResolveExprNames(&sNC, p.Where) ||
       sqlite3ResolveExprNames(&sNC, p.pHaving)
    ){
      return WRC_Abort;
    }

    /* The ORDER BY and GROUP BY clauses may not refer to terms in
    ** outer queries 
    */
    sNC.Next = 0;
    sNC.Flags |= NC_AllowAgg;

    /* Process the ORDER BY clause for singleton SELECT statements.
    ** The ORDER BY clause for compounds SELECT statements is handled
    ** below, after all of the result-sets for all of the elements of
    ** the compound have been resolved.
    */
    if( !isCompound && !sNC.resolveOrderGroupBy(p, p.pOrderBy, "ORDER") {
      return WRC_Abort;
    }
    if( db.mallocFailed ){
      return WRC_Abort;
    }
  
	//	Resolve the GROUP BY clause. At the same time, make sure the GROUP BY clause does not contain aggregate functions.
	if pGroupBy != nil {
		if !sNC.resolveOrderGroupBy(p, pGroupBy, "GROUP") || db.mallocFailed {
			return WRC_Abort;
		}
		for _, item := range pGroupBy.Items {
			if item.HasProperty(EP_Agg) {
				pParse.SetErrorMsg("aggregate functions are not allowed in the GROUP BY clause")
				return WRC_Abort
			}
		}
	}

	//	Advance to the next term of the compound
	p = p.pPrior
	nCompound++
  }

	//	Resolve the ORDER BY on a compound SELECT after all terms of the compound have been resolved.
	if isCompound && !pParse.resolveCompoundOrderBy(pLeftmost) {
		return WRC_Abort
	}
	return WRC_Prune
}

/*
** This routine walks an expression tree and resolves references to
** table columns and result-set columns.  At the same time, do error
** checking on function usage and set a flag if any aggregate functions
** are seen.
**
** To resolve table columns references we look for nodes (or subtrees) of the 
** form X.Y.Z or Y.Z or just Z where
**
**      X:   The name of a database.  Ex:  "main" or "temp" or
**           the symbolic name assigned to an ATTACH-ed database.
**
**      Y:   The name of a table in a FROM clause.  Or in a trigger
**           one of the special names "old" or "new".
**
**      Z:   The name of a column in table Y.
**
** The node at the root of the subtree is modified as follows:
**
**    Expr.op        Changed to TK_COLUMN
**    Expr.pTab      Points to the Table object for X.Y
**    Expr.iColumn   The column index in X.Y.  -1 for the rowid.
**    Expr.iTable    The VDBE cursor number for X.Y
**
**
** To resolve result-set references, look for expression nodes of the
** form Z (with no X and Y prefix) where the Z matches the right-hand
** size of an AS clause in the result-set of a SELECT.  The Z expression
** is replaced by a copy of the left-hand side of the result-set expression.
** Table-name and function resolution occurs on the substituted expression
** tree.  For example, in:
**
**      SELECT a+b AS x, c+d AS y FROM t1 ORDER BY x;
**
** The "x" term of the order by is replaced by "a+b" to render:
**
**      SELECT a+b AS x, c+d AS y FROM t1 ORDER BY a+b;
**
** Function calls are checked to make sure that the function is 
** defined and that the correct number of arguments are specified.
** If the function is an aggregate function, then the NC_HasAgg flag is
** set and the opcode is changed from TK_FUNCTION to TK_AGG_FUNCTION.
** If an expression contains aggregate functions then the EP_Agg
** property on the expression is set.
**
** An error message is left in pParse if anything is amiss.  The number
** if errors is returned.
*/
 int sqlite3ResolveExprNames( 
  NameContext *pNC,       /* Namespace to resolve expressions in. */
  Expr *pExpr             /* The expression to be analyzed. */
){
  byte savedHasAgg;
  Walker w;

  if( pExpr==0 ) return 0;
  savedHasAgg = pNC.Flags & NC_HasAgg;
  pNC.Flags &= ~NC_HasAgg;
  w.xExprCallback = resolveExprStep;
  w.xSelectCallback = resolveSelectStep;
  w.pParse = pNC.Parse;
  w.u.pNC = pNC;
  w.Expr(pExpr)
  if( pNC.Errors > 0 || w.pParse.nErr > 0 ){
    pExpr.SetProperty(EP_Error);
  }
  if( pNC.Flags & NC_HasAgg ){
    pExpr.SetProperty(EP_Agg);
  }else if( savedHasAgg ){
    pNC.Flags |= NC_HasAgg;
  }
  return pExpr.HasProperty(EP_Error);
}


/*
** Resolve all names in all expressions of a SELECT and in all
** decendents of the SELECT, including compounds off of p.pPrior,
** subqueries in expressions, and subqueries used as FROM clause
** terms.
**
** See sqlite3ResolveExprNames() for a description of the kinds of
** transformations that occur.
**
** All SELECT statements should have been expanded using
** sqlite3SelectExpand() prior to invoking this routine.
*/
 void sqlite3ResolveSelectNames(
  Parse *pParse,         /* The parser context */
  Select *p,             /* The SELECT statement being coded. */
  NameContext *pOuterNC  /* Name context for parent SELECT statement */
){
  Walker w;

  assert( p!=0 );
  w.xExprCallback = resolveExprStep;
  w.xSelectCallback = resolveSelectStep;
  w.pParse = pParse;
  w.u.pNC = pOuterNC;
  w.Select(p)
}
