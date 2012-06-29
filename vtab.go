/* This file contains code used to help implement virtual tables.
*/

/*
** Before a virtual table xCreate() or xConnect() method is invoked, the
** sqlite3.pVtabCtx member variable is set to point to an instance of
** this struct allocated on the stack. It is used by the implementation of 
** the sqlite3_declare_vtab() and vtab_config() APIs, both of which
** are invoked only from within xCreate and xConnect methods.
*/
type VtabCtx struct {
	*Table
	*VTable
}
struct VtabCtx {
  Table *pTab;
  VTable *pVTable;
};

//	The actual function that does the work of creating a new module. This function implements the sqlite3_create_module() and create_module_v2() interfaces.
func (db *sqlite3) createModule(Name string, module *sqlite3_module, parameter interface{}, xDestroy func(interface{}) interface{}) (rc int) {
//				void *pAux,						//	Context pointer for xCreate/xConnect
	db.mutex.CriticalSection(func() {
		pMod := &Module{
			Callbacks:		module,
			Parameter:		parameter,
			xDestroy:		xDestroy,
			Name:			Name,
		}
		if pDel := db.Modules[zCopy]; pDel != nil {
			if pDel.xDestroy != nil {
				db.ResetInternalSchema(-1)
				pDel.xDestroy(pDel.Parameter)
			}
		}
		rc = db.ApiExit(SQLITE_OK)
	})
	return rc
}

//	External API function used to create a new virtual-table module.
func (db *sqlite3) create_module(Name string, pModule *sqlite3_module, pAux interface{}) {
	return db.createModule(Name, pModule, pAux, 0)
}

//	External API function used to create a new virtual-table module.
func (db *sqlite3) create_module_v2(Name string, pModule *sqlite3_module, pAux interface{}, xDestroy func(interface{}) interface{}) {
	return db.createModule(Name, pModule, pAux, xDestroy)
}

//	Lock the virtual table so that it cannot be disconnected. Locks nest. Every lock should have a corresponding unlock. If an unlock is omitted, resources leaks will occur.  
//	If a disconnect is attempted while a virtual table is locked, the disconnect is deferred until all locks have been removed.
func (p *VTable) Lock() {
	p.nRef++
}

//	pTab is a pointer to a Table structure representing a virtual-table. Return a pointer to the VTable object used by connection db to access this virtual-table, if one has been created, or NULL otherwise.
func (db *sqlite3) GetVTable(pTab *Table) (pVTab *VTable) {
VTable *sqlite3GetVTable(sqlite3 *db, Table *pTab){
	assert( pTab.IsVirtual() )
	for(pVtab=pTab.VirtualTables; pVtab && pVtab.db!=db; pVtab=pVtab.Next);
	return pVtab;
}

//	Decrement the ref-count on a virtual table object. When the ref-count reaches zero, call the xDisconnect() method to delete the object.
func (pVTab *VTable) Unlock(){
	db := pVTab.db

	assert( db )
	assert( pVTab.nRef > 0 )
	assert( db.SafetyCheckOk() )

	pVTab.nRef--
	if pVTab.nRef == 0 {
		if p := pVTab.pVtab; p != nil {
			p.Callbacks.xDisconnect(p)
		}
		pVTab = nil
	}
}

//	Table p is a virtual table. This function moves all elements in the p.VirtualTables list to the sqlite3.pDisconnect lists of their associated database connections to be disconnected at the next opportunity. Except, if argument db is not nil, then the entry associated with connection db is left in the p.VirtualTables list.
func (db *sqlite3) vtabDisconnectAll(p *Table) (pRet *VTable) {
	pVTable := p.VirtualTables
	p.VirtualTables = nil
	for pVTable != nil {
		db2 := pVTable.db
		Next := pVTable.Next
		assert( db2 != nil )
		if db2 == db {
			pRet = pVTable
			p.VirtualTables = pRet
			pRet.Next = nil
		} else {
			db2.pDisconnect = append(db2.pDisconnect, pVTable)
		}
		pVTable = Next
	}
	assert( db == nil || pRet != nil )
	return
}


//	Disconnect all the virtual table objects in the sqlite3.pDisconnect list.
//	This function may only be called when the mutexes associated with all shared b-tree databases opened using connection db are held by the caller. This is done to protect the sqlite3.pDisconnect list. The sqlite3.pDisconnect list is accessed only as follows:
//		1) By this function. In this case, all BtShared mutexes and the mutex associated with the database handle itself must be held.
//		2) By function vtabDisconnectAll(), when it adds a VTable entry to the sqlite3.pDisconnect list. In this case either the BtShared mutex associated with the database the virtual table is stored in is held or, if the virtual table is stored in a non-sharable database, then the database handle mutex is held.
//	As a result, a sqlite3.pDisconnect cannot be accessed simultaneously by multiple threads. It is thread-safe.
func (db *sqlite3) VtabUnlockList(){
	if p = db.pDisconnect; p != nil {
		db.pDisconnect = nil
		db.ExpirePreparedStatements()
		for _, vtable := range p {
			vtable.Unlock()
		}
	}
}

//	Clear any and all virtual-table information from the Table record. This routine is called, for example, just before deleting the Table record.
//	Since it is a virtual-table, the Table structure contains a pointer to the head of a linked list of VTable structures. Each VTable structure is associated with a single sqlite3* user of the schema. The reference count of the VTable structure associated with database connection db is decremented immediately (which may lead to the structure being xDisconnected and free). Any other VTable structures in the list are moved to the sqlite3.pDisconnect list of the associated database connection.
func (db *sqlite3) VtabClear(Table *p) {
	if db == nil {
		db.vtabDisconnectAll(p)
	}
	p.azModuleArg = nil
}

//	Add a new module argument to pTable.azModuleArg[].
//	The string is not copied - the pointer is stored. The string will be freed automatically when the table is deleted.
static void addModuleArgument(sqlite3 *db, Table *pTable, char *zArg){
  int i = pTable.nModuleArg++;
  int nBytes = sizeof(char *)*(1+pTable.nModuleArg);
  char **azModuleArg;
  azModuleArg = sqlite3DbRealloc(db, pTable.azModuleArg, nBytes);
  if( azModuleArg==0 ){
    int j;
    for(j=0; j<i; j++){
      pTable.azModuleArg[j] = nil
    }
    zArg = nil
    pTable.azModuleArg = nil
    pTable.nModuleArg = 0;
  }else{
    azModuleArg[i] = zArg;
    azModuleArg[i+1] = 0;
  }
  pTable.azModuleArg = azModuleArg;
}

//	The parser calls this routine when it first sees a CREATE VIRTUAL TABLE statement. The module name has been parsed, but the optional list of parameters that follow the module name are still pending.
func (pParse *Parse) VtabBeginParse(pName1, pName2, pModuleName Token, ifNotExists bool) {
	sqlite3StartTable(pParse, pName1, pName2, 0, 0, 1, ifNotExists)
	if table := pParse.pNewTable; table != nil {
		assert( table.Indices != nil )

		db := pParse.db
		iDb := db.SchemaToIndex(pTable.Schema)
		assert( iDb >= 0 )

		table.tabFlags |= TF_Virtual
		table.nModuleArg = 0
		addModuleArgument(db, table, Dequote(pModuleName))
		addModuleArgument(db, table, sqlite3DbStrDup(db, db.Databases[iDb].Name))
		addModuleArgument(db, table, sqlite3DbStrDup(db, table.Name))
		pParse.sNameToken.n = (int)(&pModuleName.z[pModuleName.n] - pName1.z)

		//	Creating a virtual table invokes the authorization callback twice. The first invocation, to obtain permission to INSERT a row into the sqlite_master table, has already been made by sqlite3::StartTable(). The second call, to obtain permission to create the table, is made now.
		if table.azModuleArg {
			pParse.AuthCheck(SQLITE_CREATE_VTABLE, table.Name, table.azModuleArg[0], pParse.db.Databases[iDb].Name)
		}
	}
}

/*
** This routine takes the module argument that has been accumulating
** in pParse.zArg[] and appends it to the list of arguments on the
** virtual table currently under construction in pParse.pTable.
*/
static void addArgumentToVtab(Parse *pParse){
  if( pParse.sArg.z && pParse.pNewTable ){
    const char *z = (const char*)pParse.sArg.z;
    int n = pParse.sArg.n;
    sqlite3 *db = pParse.db;
    addModuleArgument(db, pParse.pNewTable, sqlite3DbStrNDup(db, z, n));
  }
}

//	The parser calls this routine after the CREATE VIRTUAL TABLE statement has been completely parsed.
func (pParse *Parse) VtabFinishParse(pEnd string) {
	db := pParse.db
	if pTab := pParse.pNewTable; pTab != nil {
		addArgumentToVtab(pParse)
		pParse.sArg.z = 0
		if pTab.nModuleArg > 0 {
			//	If the CREATE VIRTUAL TABLE statement is being entered for the first time (in other words if the virtual table is actually being created now instead of just being read out of sqlite_master) then do additional initialization work and store the statement text in the sqlite_master table.
			if !db.init.busy {
				//	Compute the complete text of the CREATE VIRTUAL TABLE statement
				if pEnd != "" {
					pParse.sNameToken.n = (int)(pEnd.z - pParse.sNameToken.z) + pEnd.n
				}
				statement := fmt.Sprintf("CREATE VIRTUAL TABLE %v", pParse.sNameToken)

				//	A slot for the record has already been allocated in the SQLITE_MASTER table. We just need to update that slot with all the information we've collected.  
				//	The VM register number pParse.regRowid holds the rowid of an entry in the sqlite_master table tht was created for this vtab by sqlite3StartTable().
				iDb := db.SchemaToIndex(pTab.Schema)
				sqlite3NestedParse(pParse, "UPDATE %Q.%s SET type='table', name=%Q, tbl_name=%Q, rootpage=0, sql=%Q WHERE rowid=#%d", db.Databases[iDb].Name, SCHEMA_TABLE(iDb), pTab.Name, pTab.Name, statement, pParse.regRowid)
				v := pParse.GetVdbe()
				sqlite3ChangeCookie(pParse, iDb)

				v.AddOp2(OP_Expire, 0, 0)
				where := fmt.Sprintf("name='%v' AND type='table'", pTab.Name)
				v.AddParseSchemaOp(iDb, where)
				sqlite3VdbeAddOp4(v, OP_VCreate, iDb, 0, 0, pTab.Name, sqlite3Strlen30(pTab.Name) + 1)
			} else {
				//	If we are rereading the sqlite_master table create the in-memory record of the table. The xConnect() method is not called until the first time the virtual table is used in an SQL statement. This allows a schema that contains virtual tables to be loaded before the required virtual table implementations are registered.
				pTab.Schema.schema.Tables[pTab.Name] = pTab
				pParse.pNewTable = nil
			}
		}
	}
}

/*
** The parser calls this routine when it sees the first token
** of an argument to the module name in a CREATE VIRTUAL TABLE statement.
*/
 void sqlite3VtabArgInit(Parse *pParse){
  addArgumentToVtab(pParse);
  pParse.sArg.z = 0;
  pParse.sArg.n = 0;
}

/*
** The parser calls this routine for each token after the first token
** in an argument to the module name in a CREATE VIRTUAL TABLE statement.
*/
 void sqlite3VtabArgExtend(Parse *pParse, Token *p){
  Token *pArg = &pParse.sArg;
  if( pArg.z==0 ){
    pArg.z = p.z;
    pArg.n = p.n;
  }else{
    assert(pArg.z < p.z);
    pArg.n = (int)(&p.z[p.n] - pArg.z);
  }
}

/*
** Invoke a virtual table constructor (either xCreate or xConnect). The
** pointer to the function to invoke is passed as the fourth parameter
** to this procedure.
*/
static int vtabCallConstructor(
  sqlite3 *db, 
  Table *pTab,
  Module *pMod,
  int (*xConstruct)(sqlite3*,void*,int,const char*const*,sqlite3_vtab**,char**),
  char **pzErr
){
  VtabCtx sCtx, *pPriorCtx;
  VTable *pVTable;
  int rc;
  const char *const*azArg = (const char *const*)pTab.azModuleArg;
  int nArg = pTab.nModuleArg;
  char *zErr = 0;
  char *zModuleName = fmt.Sprintf("%v", pTab.Name);

  if( !zModuleName ){
    return SQLITE_NOMEM;
  }

  pVTable = sqlite3DbMallocZero(db, sizeof(VTable));
  if( !pVTable ){
    zModuleName = nil
    return SQLITE_NOMEM;
  }
  pVTable.db = db;
  pVTable.Module = pMod;

  /* Invoke the virtual table constructor */
  assert( &db.pVtabCtx );
  assert( xConstruct );
  sCtx.pTab = pTab;
  sCtx.VirtualTables = pVTable;
  pPriorCtx = db.pVtabCtx;
  db.pVtabCtx = &sCtx;
  rc = xConstruct(db, pMod.Parameter, nArg, azArg, &pVTable.pVtab, &zErr);
  db.pVtabCtx = pPriorCtx;
  if( rc==SQLITE_NOMEM ) db.mallocFailed = true

  if( SQLITE_OK!=rc ){
    if( zErr==0 ){
      *pzErr = fmt.Sprintf("vtable constructor failed: %v", zModuleName);
    }else {
      *pzErr = fmt.Sprintf("%v", zErr);
      zErr = nil
    }
    pVTable = nil
  }else if pVTable.pVtab {
    pVTable.pVtab.Callbacks = pMod.Callbacks;
    pVTable.nRef = 1;
    if( sCtx.pTab ){
      const char *zFormat = "vtable constructor did not declare schema: %s";
      *pzErr = fmt.Sprintf(zFormat, pTab.Name);
      pVTable.Unlock()
      rc = SQLITE_ERROR;
    }else{
      int iCol;
      //	If everything went according to plan, link the new VTable structure into the linked list headed by pTab.VirtualTables. Then loop through the columns of the table to see if any of them contain the token "hidden". If so, set the Column.IsHidden flag and remove the token from the type string.
      pVTable.Next = pTab.VirtualTables;
      pTab.VirtualTables = pVTable;

      for(iCol=0; iCol<pTab.nCol; iCol++){
        char *zType = pTab.Columns[iCol].zType;
        int nType;
        int i = 0;
        if( !zType ) continue;
        nType = sqlite3Strlen30(zType);
        if !CaseInsensitiveMatchN("hidden", zType, 6)||(zType[6] && zType[6]!=' ') {
          for(i=0; i<nType; i++){
            if CaseInsensitiveMatchN(" hidden", &zType[i], 7) && (zType[i+7]=='\0' || zType[i+7]==' ') {
              i++;
              break;
            }
          }
        }
        if( i<nType ){
          int j;
          int nDel = 6 + (zType[i+6] ? 1 : 0);
          for(j=i; (j+nDel)<=nType; j++){
            zType[j] = zType[j+nDel];
          }
          if( zType[i]=='\0' && i>0 ){
            assert(zType[i-1]==' ');
            zType[i-1] = '\0';
          }
          pTab.Columns[iCol].IsHidden = true
        }
      }
    }
  }

  zModuleName = nil
  return rc;
}

/*
** This function is invoked by the parser to call the xConnect() method
** of the virtual table pTab. If an error occurs, an error code is returned 
** and an error left in pParse.
**
** This call is a no-op if table pTab is not a virtual table.
*/
 int sqlite3VtabCallConnect(Parse *pParse, Table *pTab){
  sqlite3 *db = pParse.db;
  const char *zMod;
  Module *pMod;
  int rc;

  assert( pTab );
  if( (pTab.tabFlags & TF_Virtual)==0 || sqlite3GetVTable(db, pTab) ){
    return SQLITE_OK;
  }

  /* Locate the required virtual table module */
  zMod = pTab.azModuleArg[0];
  pMod = db.Modules[zMod]

  if( !pMod ){
    const char *zModule = pTab.azModuleArg[0];
    pParse.SetErrorMsg("no such module: %v", zModule);
    rc = SQLITE_ERROR;
  }else{
    char *zErr = 0;
    rc = vtabCallConstructor(db, pTab, pMod, pMod.Callbacks.xConnect, &zErr);
    if( rc!=SQLITE_OK ){
      pParse.SetErrorMsg("%v", zErr);
    }
    zErr = nil
  }

  return rc;
}
/*
** Grow the db.aVTrans[] array so that there is room for at least one
** more v-table. Return SQLITE_NOMEM if a malloc fails, or SQLITE_OK otherwise.
*/
static int growVTrans(sqlite3 *db){
  const int ARRAY_INCR = 5;

  /* Grow the sqlite3.aVTrans array if required */
  if( (db.nVTrans%ARRAY_INCR)==0 ){
    VTable **aVTrans;
    int nBytes = sizeof(sqlite3_vtab *) * (db.nVTrans + ARRAY_INCR);
    aVTrans = sqlite3DbRealloc(db, (void *)db.aVTrans, nBytes);
    if( !aVTrans ){
      return SQLITE_NOMEM;
    }
    memset(&aVTrans[db.nVTrans], 0, sizeof(sqlite3_vtab *)*ARRAY_INCR);
    db.aVTrans = aVTrans;
  }

  return SQLITE_OK;
}

/*
** Add the virtual table pVTab to the array sqlite3.aVTrans[]. Space should
** have already been reserved using growVTrans().
*/
static void addToVTrans(sqlite3 *db, VTable *pVTab){
  /* Add pVtab to the end of sqlite3.aVTrans */
  db.aVTrans[db.nVTrans++] = pVTab;
  pVTab.Lock()
}

/*
** This function is invoked by the vdbe to call the xCreate method
** of the virtual table named zTab in database iDb. 
**
** If an error occurs, *pzErr is set to point an an English language
** description of the error and an SQLITE_XXX error code is returned.
*/
 int sqlite3VtabCallCreate(sqlite3 *db, int iDb, const char *zTab, char **pzErr){
  int rc = SQLITE_OK;
  Table *pTab;
  Module *pMod;
  const char *zMod;

  pTab = db.FindTable(zTab, db.Databases[iDb].Name)
  assert( pTab && (pTab.tabFlags & TF_Virtual)!=0 && !pTab.VirtualTables );

  /* Locate the required virtual table module */
  zMod = pTab.azModuleArg[0];
  pMod = db.Modules[zMod]

  /* If the module has been registered and includes a Create method, 
  ** invoke it now. If the module has not been registered, return an 
  ** error. Otherwise, do nothing.
  */
  if( !pMod ){
    *pzErr = fmt.Sprintf("no such module: %v", zMod);
    rc = SQLITE_ERROR;
  }else{
    rc = vtabCallConstructor(db, pTab, pMod, pMod.Callbacks.xCreate, pzErr);
  }

  if rc == SQLITE_OK && sqlite3GetVTable(db, pTab) {
    rc = growVTrans(db);
    if( rc==SQLITE_OK ){
      addToVTrans(db, sqlite3GetVTable(db, pTab));
    }
  }

  return rc;
}

/*
** This function is used to set the schema of a virtual table.  It is only
** valid to call this function from within the xCreate() or xConnect() of a
** virtual table module.
*/
func int sqlite3_declare_vtab(sqlite3 *db, const char *zCreateTable){
  Parse *pParse;

  int rc = SQLITE_OK;
  Table *pTab;
  char *zErr = 0;

  db.mutex.Lock()
  if !db.pVtabCtx || !(pTab = db.pVtabCtx.pTab) {
    db.Error(SQLITE_MISUSE, "");
    db.mutex.Unlock()
    return SQLITE_MISUSE_BKPT;
  }
  assert( (pTab.tabFlags & TF_Virtual)!=0 );

  pParse = sqlite3StackAllocZero(db, sizeof(*pParse));
  if( pParse==0 ){
    rc = SQLITE_NOMEM;
  }else{
    pParse.declareVtab = 1;
    pParse.db = db;
    pParse.nQueryLoop = 1;
  
    if pParse.Run(zCreateTable, &zErr) == SQLITE_OK && pParse.pNewTable != nil && !db.mallocFailed && !pParse.pNewTable.Select && pParse.pNewTable.tabFlags & TF_Virtual == 0 {
      if( !pTab.Columns ){
        pTab.Columns = pParse.pNewTable.Columns;
        pTab.nCol = pParse.pNewTable.nCol;
        pParse.pNewTable.nCol = 0;
        pParse.pNewTable.Columns = 0;
      }
      db.pVtabCtx.pTab = nil
    }else{
      db.Error(SQLITE_ERROR, (zErr ? "%s" : 0), zErr);
      zErr = nil
      rc = SQLITE_ERROR;
    }
    pParse.declareVtab = 0;
  
    if( pParse.pVdbe ){
      sqlite3VdbeFinalize(pParse.pVdbe);
    }
    db.DeleteTable(pParse.pNewTable)
	pParse.pNewTable = nil
    sqlite3StackFree(db, pParse)
  }

  assert( (rc&0xff)==rc );
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

//	This function is invoked by the vdbe to call the xDestroy method of the virtual table named zTab in database iDb. This occurs when a DROP TABLE is mentioned.
//	This call is a no-op if zTab is not a virtual table.
func (db *sqlite3) VtabCallDestroy(iDb int, zTab string) (rc int) {
	if pTab := db.FindTable(zTab, db.Databases[iDb].Name); pTab != nil && pTab.VirtualTables != nil {
		p := db.vtabDisconnectAll(pTab)
		assert( rc == SQLITE_OK )
		if rc = p.Module.Callbacks.xDestroy(p.pVtab); rc == SQLITE_OK {
			//	Remove the sqlite3_vtab* from the aVTrans[] array, if applicable
			assert( pTab.VirtualTables == p && p.Next == nil )
			p.pVtab = nil
			pTab.VirtualTables = nil
			pVTab.Unlock()
		}
	}
	return rc
}

//	This function invokes either the xRollback or xCommit method of each of the virtual tables in the sqlite3.aVTrans array. The method called is identified by the second argument, "offset", which is the offset of the method to call in the sqlite3_module structure.
//	The array is cleared after invoking the callbacks. 
func (db *sqlite3) CallFinaliser(offset int) {
	if db.aVTrans != nil {
		for i := 0; i < db.nVTrans; i++ {
			pVTab := db.aVTrans[i]
			if p := pVTab.pVtab; p != nil {
				if x := *(int (**)(sqlite3_vtab *))((char *)p.Callbacks + offset); x != nil {
					x(p)
				}
			}
			pVTab.SavepointStackDepth = 0
			pVTab.Unlock()
		}
		db.nVTrans = 0
		db.aVTrans = nil
	}
}

/*
** Invoke the xSync method of all virtual tables in the sqlite3.aVTrans
** array. Return the error code for the first error that occurs, or
** SQLITE_OK if all xSync operations are successful.
*/
 int sqlite3VtabSync(sqlite3 *db, char **pzErrmsg){
  int i;
  int rc = SQLITE_OK;
  VTable **aVTrans = db.aVTrans;

  db.aVTrans = 0;
  for(i=0; rc==SQLITE_OK && i<db.nVTrans; i++){
    int (*x)(sqlite3_vtab *);
    sqlite3_vtab *pVtab = aVTrans[i].pVtab;
    if( pVtab && (x = pVtab.Callbacks.xSync)!=0 ){
      rc = x(pVtab);
      *pzErrmsg = sqlite3DbStrDup(db, pVtab.zErrMsg);
      pVtab.zErrMsg = nil
    }
  }
  db.aVTrans = aVTrans;
  return rc;
}

//	Invoke the xRollback method of all virtual tables in the sqlite3.aVTrans array. Then clear the array itself.
func (db *sqlite3) VtabRollback() int {
	db.CallFinaliser(offsetof(sqlite3_module, xRollback))
	return SQLITE_OK
}

//	Invoke the xCommit method of all virtual tables in the sqlite3.aVTrans array. Then clear the array itself.
func (db *sqlite3) VtabCommit() int {
	db.CallFinaliser(offsetof(sqlite3_module, xCommit))
	return SQLITE_OK
}

/*
** If the virtual table pVtab supports the transaction interface
** (xBegin/xRollback/xCommit and optionally xSync) and a transaction is
** not currently open, invoke the xBegin method now.
**
** If the xBegin call is successful, place the sqlite3_vtab pointer
** in the sqlite3.aVTrans array.
*/
 int sqlite3VtabBegin(sqlite3 *db, VTable *pVTab){
  int rc = SQLITE_OK;
  const sqlite3_module *pModule;

  /* Special case: If db.aVTrans is NULL and db.nVTrans is greater
  ** than zero, then this function is being called from within a
  ** virtual module xSync() callback. It is illegal to write to 
  ** virtual module tables in this case, so return SQLITE_LOCKED.
  */
  if db.nVTrans > 0 && db.aVTrans == 0 {
    return SQLITE_LOCKED;
  }
  if( !pVTab ){
    return SQLITE_OK;
  } 
  pModule = pVTab.pVtab.Callbacks;

  if( pModule.xBegin ){
    int i;

    /* If pVtab is already in the aVTrans array, return early */
    for(i=0; i<db.nVTrans; i++){
      if( db.aVTrans[i]==pVTab ){
        return SQLITE_OK;
      }
    }

    /* Invoke the xBegin method. If successful, add the vtab to the 
    ** sqlite3.aVTrans[] array. */
    rc = growVTrans(db);
    if( rc==SQLITE_OK ){
      rc = pModule.xBegin(pVTab.pVtab);
      if( rc==SQLITE_OK ){
        addToVTrans(db, pVTab);
      }
    }
  }
  return rc;
}

/*
** Invoke either the xSavepoint, xRollbackTo or xRelease method of all
** virtual tables that currently have an open transaction. Pass iSavepoint
** as the second argument to the virtual table method invoked.
**
** If op is SAVEPOINT_BEGIN, the xSavepoint method is invoked. If it is
** SAVEPOINT_ROLLBACK, the xRollbackTo method. Otherwise, if op is 
** SAVEPOINT_RELEASE, then the xRelease method of each virtual table with
** an open transaction is invoked.
**
** If any virtual table method returns an error code other than SQLITE_OK, 
** processing is abandoned and the error returned to the caller of this
** function immediately. If all calls to virtual table methods are successful,
** SQLITE_OK is returned.
*/
 int sqlite3VtabSavepoint(sqlite3 *db, int op, int iSavepoint){
  int rc = SQLITE_OK;

  assert( op==SAVEPOINT_RELEASE||op==SAVEPOINT_ROLLBACK||op==SAVEPOINT_BEGIN );
  assert( iSavepoint>=0 );
  if( db.aVTrans ){
    int i;
    for(i=0; rc==SQLITE_OK && i<db.nVTrans; i++){
      VTable *pVTab = db.aVTrans[i];
      const sqlite3_module *pMod = pVTab.Module.Callbacks;
      if( pVTab.pVtab && pMod.iVersion>=2 ){
        int (*xMethod)(sqlite3_vtab *, int);
        switch( op ){
          case SAVEPOINT_BEGIN:
            xMethod = pMod.xSavepoint;
            pVTab.SavepointStackDepth = iSavepoint+1;
            break;
          case SAVEPOINT_ROLLBACK:
            xMethod = pMod.xRollbackTo;
            break;
          default:
            xMethod = pMod.xRelease;
            break;
        }
        if( xMethod && pVTab.SavepointStackDepth>iSavepoint ){
          rc = xMethod(pVTab.pVtab, iSavepoint);
        }
      }
    }
  }
  return rc;
}

/*
** The first parameter (pDef) is a function implementation.  The
** second parameter (pExpr) is the first argument to this function.
** If pExpr is a column in a virtual table, then let the virtual
** table implementation have an opportunity to overload the function.
**
** This routine is used to allow virtual table implementations to
** overload MATCH, LIKE, GLOB, and REGEXP operators.
**
** Return either the pDef argument (indicating no change) or a 
** new FuncDef structure that is marked as ephemeral using the
** SQLITE_FUNC_EPHEM flag.
*/
 FuncDef *sqlite3VtabOverloadFunction(
  sqlite3 *db,    /* Database connection for reporting malloc problems */
  FuncDef *pDef,  /* Function to possibly overload */
  int nArg,       /* Number of arguments to the function */
  Expr *pExpr     /* First argument to the function */
){
  Table *pTab;
  sqlite3_vtab *pVtab;
  sqlite3_module *pMod;
  void (*xFunc)(sqlite3_context*,int,sqlite3_value**) = 0;
  void *pArg = 0;
  FuncDef *pNew;
  int rc = 0;
  char *zLowerName;
  unsigned char *z;


  /* Check to see the left operand is a column in a virtual table */
  if( pExpr==0 ) return pDef;
  if( pExpr.op!=TK_COLUMN ) return pDef;
  pTab = pExpr.pTab;
  if( pTab==0 ) return pDef;
  if( (pTab.tabFlags & TF_Virtual)==0 ) return pDef;
  pVtab = sqlite3GetVTable(db, pTab).pVtab;
  assert( pVtab!=0 );
  assert( pVtab.Callbacks!=0 );
  pMod = (sqlite3_module *)pVtab.Callbacks;
  if( pMod.xFindFunction==0 ) return pDef;
 
  /* Call the xFindFunction method on the virtual table implementation
  ** to see if the implementation wants to overload this function 
  */
  zLowerName = sqlite3DbStrDup(db, pDef.Name);
  if( zLowerName ){
    for(z=(unsigned char*)zLowerName; *z; z++){
      *z = sqlite3UpperToLower[*z];
    }
    rc = pMod.xFindFunction(pVtab, nArg, zLowerName, &xFunc, &pArg);
    zLowerName = nil
  }
  if( rc==0 ){
    return pDef;
  }

  /* Create a new ephemeral function definition for the overloaded
  ** function */
  pNew = sqlite3DbMallocZero(db, sizeof(*pNew)
                             + sqlite3Strlen30(pDef.Name) + 1);
  if( pNew==0 ){
    return pDef;
  }
  *pNew = *pDef;
  pNew.Name = (char *)&pNew[1];
  memcpy(pNew.Name, pDef.Name, sqlite3Strlen30(pDef.Name)+1);
  pNew.xFunc = xFunc;
  pNew.pUserData = pArg;
  pNew.flags |= SQLITE_FUNC_EPHEM;
  return pNew;
}

/*
** Make sure virtual table pTab is contained in the pParse.apVirtualLock[]
** array so that an OP_VBegin will get generated for it.  Add pTab to the
** array if it is missing.  If pTab is already in the array, this routine
** is a no-op.
*/
 void sqlite3VtabMakeWritable(Parse *pParse, Table *pTab){
  pToplevel := pParse.Toplevel()
  int i, n;
  Table **apVtabLock;

  assert( pTab.IsVirtual() );
  for(i=0; i<pToplevel.nVtabLock; i++){
    if( pTab==pToplevel.apVtabLock[i] ) return;
  }
  n = (pToplevel.nVtabLock+1)*sizeof(pToplevel.apVtabLock[0]);
  apVtabLock = sqlite3_realloc(pToplevel.apVtabLock, n);
  if( apVtabLock ){
    pToplevel.apVtabLock = apVtabLock;
    pToplevel.apVtabLock[pToplevel.nVtabLock++] = pTab;
  }else{
    pToplevel.db.mallocFailed = true
  }
}

/*
** Return the ON CONFLICT resolution mode in effect for the virtual
** table update operation currently in progress.
**
** The results of this routine are undefined unless it is called from
** within an xUpdate method.
*/
func int sqlite3_vtab_on_conflict(sqlite3 *db){
  static const unsigned char aMap[] = { 
    SQLITE_ROLLBACK, SQLITE_ABORT, SQLITE_FAIL, SQLITE_IGNORE, SQLITE_REPLACE 
  };
  assert( OE_Rollback==1 && OE_Abort==2 && OE_Fail==3 );
  assert( OE_Ignore==4 && OE_Replace==5 );
  assert( db.vtabOnConflict>=1 && db.vtabOnConflict<=5 );
  return (int)aMap[db.vtabOnConflict-1];
}

//	Call from within the xCreate() or xConnect() methods to provide the SQLite core with additional information about the behavior of the virtual table being implemented.
func (db *sqlite3) vtab_config(op int, args... interface{}) (rc int) {
	rc = SQLITE_MISUSE_BKPT
	db.mutex.CriticalSection(func() {
		if op == SQLITE_VTAB_CONSTRAINT_SUPPORT {
			if p := db.pVtabCtx; p != nil {
				assert( p.pTab == nil || (p.pTab.tabFlags & TF_Virtual) != 0 )
				p.VTable.SupportsConstraints = args[0].(bool)
			}
			rc = SQLITE_OK
		}
		if rc != SQLITE_OK {
			db.Error(rc, "")
		}
	})
	return
}