//	This file contains code used to implement the sqlite3_set_authorizer() API.

//	Set or clear the access authorization function.
//	The access authorization function is be called during the compilation phase to verify that the user has read and/or write access permission on various fields of the database. The first argument to the auth function is a copy of the 3rd argument to this routine. The second argument to the auth function is one of these constants:
//			SQLITE_CREATE_INDEX
//			SQLITE_CREATE_TABLE
//			SQLITE_CREATE_TEMP_INDEX
//			SQLITE_CREATE_TEMP_TABLE
//			SQLITE_CREATE_TEMP_TRIGGER
//			SQLITE_CREATE_TEMP_VIEW
//			SQLITE_CREATE_TRIGGER
//			SQLITE_CREATE_VIEW
//			SQLITE_DELETE
//			SQLITE_DROP_INDEX
//			SQLITE_DROP_TABLE
//			SQLITE_DROP_TEMP_INDEX
//			SQLITE_DROP_TEMP_TABLE
//			SQLITE_DROP_TEMP_TRIGGER
//			SQLITE_DROP_TEMP_VIEW
//			SQLITE_DROP_TRIGGER
//			SQLITE_DROP_VIEW
//			SQLITE_INSERT
//			SQLITE_PRAGMA
//			SQLITE_READ
//			SQLITE_SELECT
//			SQLITE_TRANSACTION
//			SQLITE_UPDATE
//	The third and fourth arguments to the auth function are the name of the table and the column that are being accessed. The auth function should return either SQLITE_OK, SQLITE_DENY, or SQLITE_IGNORE. If SQLITE_OK is returned, it means that access is allowed. SQLITE_DENY means that the SQL statement will never-run - the sqlite3_exec() call will return with an error. SQLITE_IGNORE means that the SQL statement should run but attempts to read the specified column will return nil and attempts to write the column will be ignored.
//	Setting the auth function to nil disables this hook. The default setting of the auth function is nil.
func int sqlite3_set_authorizer(
  sqlite3 *db,
  int (*xAuth)(void*,int,const char*,const char*,const char*,const char*),
  void *pArg
){
	db.mutex.CriticalSection(func() {
		db.xAuth = xAuth
		db.pAuthArg = pArg
		db.ExpirePreparedStatements()
	})
	return SQLITE_OK;
}

//	Write an error message into pParse.zErrMsg that explains that the user-supplied authorization function returned an illegal value.
func (pParse *Parse) AuthBadReturnCode() {
	pParse.SetErrorMsg("authorizer malfunction")
	pParse.rc = SQLITE_ERROR
}

//	Invoke the authorization callback for permission to read column zCol from table zTab in database zDb. This function assumes that an authorization callback has been registered (i.e. that sqlite3.xAuth is not NULL).
//	If SQLITE_IGNORE is returned and pExpr is not NULL, then pExpr is changed to an SQL NULL expression. Otherwise, if pExpr is NULL, then SQLITE_IGNORE is treated as SQLITE_DENY. In this case an error is left in pParse.
func (pParse *Parse) AuthReadColumn(table, column string, iDb int) (rc int)
	db := pParse.db
	database := db.Databases[iDb].Name

	switch rc = db.xAuth(db.pAuthArg, SQLITE_READ, table, column, database, pParse.zAuthContext); {
	case rc == SQLITE_DENY:
		if len(db.Databases) > 2 || iDb != 0 {
			pParse.SetErrorMsg("access to %v.%v.%v is prohibited", database, table, column)
		} else {
			pParse.SetErrorMsg("access to %v.%v is prohibited", table, column)
		}
		pParse.rc = SQLITE_AUTH
	case rc != SQLITE_IGNORE && rc != SQLITE_OK {
		pParse.AuthBadReturnCode()
	}
	return rc
}

//	The pExpr should be a TK_COLUMN expression. The table referred to is in pTabList or else it is the NEW or OLD table of a trigger. Check to see if it is OK to read this particular column.
//	If the auth function returns SQLITE_IGNORE, change the TK_COLUMN instruction into a TK_NULL. If the auth function returns SQLITE_DENY, then generate an error.
func (pParse *Parse) AuthRead(pExpr *Expr, schema *Schema, pTabList *SrcList) {
	if db := pParse.db; db.xAuth != nil {
		iDb := pParse.db.SchemaToIndex(schema)
		if iDb < 0 {
			//	An attempt to read a column out of a subquery or other temporary table.
			return
		}

		assert( pExpr.op == TK_COLUMN || pExpr.op == TK_TRIGGER )

		pTab	*Table
		if pExpr.op == TK_TRIGGER {
			pTab = pParse.pTriggerTab
		} else {
			for _, table := range pTablist {
				if pExpr.iTable == table.iCursor {
					pTab = table.pTab
					break
				}
	    	}
		}
		if pTab != nil {
			iCol := pExpr.iColumn
			zCol := ""
			if iCol >= 0 {
				assert( iCol < pTab.nCol )
				zCol = pTab.Columns[iCol].Name
			} else if pTab.iPKey >= 0 {
				assert( pTab.iPKey < pTab.nCol )
				zCol = pTab.Columns[pTab.iPKey].Name
			} else {
				zCol = "ROWID"
			}
			assert( iDb >= 0 && iDb < len(db.Databases) )
			if SQLITE_IGNORE == pParse.AuthReadColumn(pTab.Name, zCol, iDb) {
				pExpr.op = TK_NULL
			}
		}
	}
}

//	Do an authorization check using the code and arguments given. Return either SQLITE_OK (zero) or SQLITE_IGNORE or SQLITE_DENY. If SQLITE_DENY is returned, then the error count and error message in pParse are modified appropriately.
func (pParse *Parse) AuthCheck(code int, args... string) (rc int) {
	db := pParse.db
	//	Don't do any authorization checks if the database is initialising or if the parser is being invoked from within sqlite3_declare_vtab.
	if db.init.busy || IN_DECLARE_VTAB || db.xAuth == nil {
		return SQLITE_OK
	}

	switch rc = db.xAuth(db.pAuthArg, code, args[0], args[1], args[2], pParse.zAuthContext); {
	case rc == SQLITE_DENY:
		pParse.SetErrorMsg("not authorized")
		pParse.rc = SQLITE_AUTH
	case rc != SQLITE_OK && rc != SQLITE_IGNORE:
		rc = SQLITE_DENY
		pParse.AuthBadReturnCode()
	}
	return
}

//	Push an authorization context. After this routine is called, the zArg3 argument to authorization callbacks will be zContext until popped. Or if pParse == nil, this routine is a no-op.
func (pParse *Parse) AuthContextPush(pContext *AuthContext, zContext string) {
	assert( pParse )
	pContext.pParse = pParse
	pContext.zAuthContext = pParse.zAuthContext
	pParse.zAuthContext = zContext
}

//	Pop an authorization context that was previously pushed by sqlite3::AuthContextPush
func (pContext *Context) Pop() {
	if pContext.pParse != nil {
		pContext.pParse.zAuthContext = pContext.zAuthContext
		pContext.pParse = nil
	}
}