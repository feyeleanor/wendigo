//	The following structure contains information used by the Fix... routines as they walk the parse tree to make database references explicit.  
type DbFixer struct {
	*Parse *pParse			//	The parsing context.  Error messages written here
	Db		string			//	Make sure all objects are contained in this database
	Type	string			//	Type of the container - used for error messages
	Name	Token			//	Name of the container - used for error messages
}

//	Initialize a DbFixer structure. This routine must be called prior to passing the structure to one of the sqliteFixAAAA() routines below.
//	A return value indicates that fixation is required.
func (pParse *Parse) NewDbFixer(iDb int, Type string, Name Token) (pFix *DbFixer) {
	if iDb == 0 || iDb > 1 {
	    db := pParse.db
	    assert( len(db.Databases) > iDb )
	    pFix := &DbFixer{
			Parse:		pParse,
			Db:			db.Databases[iDb].Name,
			Type:		Type,
			Name:		Name,
		}
	}
	return
}

//	The following set of routines walk through the parse tree and assign a specific database to all table references where the database name was left unspecified in the original SQL statement. The pFix structure must have been initialized by a prior call to sqlite3FixInit().
//	These routines are used to make sure that an index, trigger, or view in one database does not refer to objects in a different database. (Exception: indices, triggers, and views in the TEMP database are allowed to refer to anything.) If a reference is explicitly made to an object in a different database, an error message is added to pParse.zErrMsg and these routines return non-zero. If everything checks out, these routines return 0.
func (pFix *DbFixer) FixSrcList(pList *SrcList) (rc int) {
	if pList != nil {
	    Db := pFix.Db
		for _, item := range pList.Items {
			if item.Database == "" {
				item.Database = Db
			} else if !CaseInsensitiveMatch(item.Database, Db) {
				pFix.pParse.SetErrorMsg("%v %v cannot reference objects in database %v", pFix.Type, pFix.Name, item.Database)
				return SQLITE_ERROR
			}
			if pFix.FixSelect(item.Select) != SQLITE_OK || pFix.FixExpr(item.pOn) {
				return SQLITE_ERROR
			}
		}
	}
	return
}

func (pFix *DbFixer) FixSelect(pSelect *Select) (rc int) {
	for ; pSelect != nil; pSelect = pSelect.pPrior {
		switch {
		case pFix.FixExprList(pSelect.pEList) != SQLITE_OK:
			return SQLITE_ERROR
		case pFix.FixSrcList(pSelect.pSrc) != SQLITE_OK:
			return SQLITE_ERROR
		case pFix.FixExpr(pSelect.Where) != SQLITE_OK:
			return SQLITE_ERROR
		case pFix.FixExpr(pSelect.pHaving) != SQLITE_OK {
			return SQLITE_ERROR
		}
	}
	return
}

func (pFix *DbFixer) FixExpr(pExpr *Expr) (rc int) {
	for ; pExpr != nil; pExpr = pExpr.pLeft {
		if pExpr.HasProperty(EP_xIsSelect) {
			if pFix.FixSelect(pExpr.x.Select) != SQLITE_OK {
				return SQLITE_ERROR
			}
		} else {
			if pFix.FixExprList(pExpr.pList) != SQLITE_OK  {
				return SQLITE_ERROR
			}
		}
		if pFix.FixExpr(pExpr.pRight) != SQLITE_OK {
			return SQLITE_ERROR
		}
	}
	return
}

func (pFix *DbFixer) FixExprList(pList *ExprList) (rc int) {
	for _, item := range pList.Items {
		if pFix.FixExpr(pFix, item.Expr) != SQLITE_OK {
			return SQLITE_ERROR
		}
	}
	return
}

func (pFix *DbFixer) FixTriggerStep(pStep *TriggerStep) (rc int) {
	for ; pStep != nil; pStep = pStep.Next {
		if pFix.FixSelect(pStep.Select) != SQLITE_OK || pFix.FixExpr(pStep.Where) != SQLITE_OK || pFix.FixExprList(pStep.ExprList) != SQLITE_OK {
			return SQLITE_ERROR
		}
	}
	return
}
