/* This file contains code used to implement the ATTACH and DETACH commands.
*/

#ifndef SQLITE_OMIT_ATTACH
/*
** Resolve an expression that was part of an ATTACH or DETACH statement. This
** is slightly different from resolving a normal SQL expression, because simple
** identifiers are treated as strings, not possible column names or aliases.
**
** i.e. if the parser sees:
**
**     ATTACH DATABASE abc AS def
**
** it treats the two expressions as literal strings 'abc' and 'def' instead of
** looking for columns of the same name.
**
** This only applies to the root node of pExpr, so the statement:
**
**     ATTACH DATABASE abc||def AS 'db2'
**
** will fail because neither abc or def can be resolved.
*/
static int resolveAttachExpr(NameContext *pName, Expr *pExpr)
{
  int rc = SQLITE_OK;
  if( pExpr ){
    if( pExpr.op!=TK_ID ){
      rc = sqlite3ResolveExprNames(pName, pExpr);
      if( rc==SQLITE_OK && !sqlite3ExprIsConstant(pExpr) ){
        pName.Parse.SetErrorMsg("invalid name: \"%v\"", pExpr.Token);
        return SQLITE_ERROR;
      }
    }else{
      pExpr.op = TK_STRING;
    }
  }
  return rc;
}

/*
** An SQL user-function registered to do the work of an ATTACH statement. The
** three arguments to the function come directly from an attach statement:
**
**     ATTACH DATABASE x AS y KEY z
**
**     SELECT sqlite_attach(x, y, z)
**
** If the optional "KEY z" syntax is omitted, an SQL NULL is passed as the
** third argument.
*/
static void attachFunc(
  sqlite3_context *context,
  int NotUsed,
  sqlite3_value **argv
){
  int i;
  int rc = 0;
  sqlite3 *db = sqlite3_context_db_handle(context);
  const char *Name;
  const char *zFile;
  char *zPath = 0;
  char *zErr = 0;
  uint flags;
  Db *aNew;
  char *zErrDyn = 0;
  sqlite3_vfs *pVfs;

  zFile = (const char *)sqlite3_value_text(argv[0]);
  Name = (const char *)sqlite3_value_text(argv[1]);
  if( zFile==0 ) zFile = "";
  if( Name==0 ) Name = "";

  /* Check for the following errors:
  **
  **     * Transaction currently open
  **     * Specified database name already being used.
  */
  if( !db.autoCommit ){
    zErrDyn = "cannot ATTACH database within transaction"
    goto attach_error;
  }
  for i, database := range db.Databases {
    z := db.Databases[i].Name
    if CaseInsensitiveMatch(z, Name) {
      zErrDyn = fmt.Sprintf("database %v is already in use", Name);
      goto attach_error;
    }
  }

  /* Allocate the new entry in the db.Databases[] array and initialise the schema
  ** hash tables.
  */
  if( db.Databases==db.DatabasesStatic ){
    aNew = sqlite3DbMallocRaw(db, sizeof(db.Databases[0])*3 );
    if( aNew==0 ) return;
    memcpy(aNew, db.Databases, sizeof(db.Databases[0])*2);
  }else{
    aNew = sqlite3DbRealloc(db, db.Databases, sizeof(db.Databases[0])*(len(db.Databases) + 1) );
    if( aNew==0 ) return;
  }
  db.Databases = aNew;
  aNew = &db.Databases[len(db.Databases)];
  memset(aNew, 0, sizeof(*aNew));

  /* Open the database file. If the btree is successfully opened, use
  ** it to obtain the database schema. At this point the schema may
  ** or may not be initialised.
  */
  flags = db.openFlags;
  rc = sqlite3ParseUri(db.pVfs.Name, zFile, &flags, &pVfs, &zPath, &zErr);
  if( rc!=SQLITE_OK ){
    if( rc==SQLITE_NOMEM ) db.mallocFailed = true
    sqlite3_result_error(context, zErr, -1);
    zErr = nil
    return;
  }
  assert( pVfs );
  flags |= SQLITE_OPEN_MAIN_DB;
  rc = sqlite3BtreeOpen(pVfs, zPath, db, &aNew.pBt, 0, flags);
  zPath = nil
  db.nDb++;
  if( rc==SQLITE_CONSTRAINT ){
    rc = SQLITE_ERROR;
    zErrDyn = "database is already attached"
  }else if( rc==SQLITE_OK ){
    Pager *pPager;
    aNew.Schema = db.GetSchema(aNew.pBt)
    if( !aNew.Schema ){
      rc = SQLITE_NOMEM;
    }else if( aNew.Schema.file_format && aNew.Schema.enc != db.Encoding() ){
      zErrDyn = "attached databases must use the same text encoding as main database"
      rc = SQLITE_ERROR;
    }
    pPager = aNew.pBt.Pager()
    sqlite3PagerLockingMode(pPager, db.dfltLockMode);
    sqlite3BtreeSecureDelete(aNew.pBt, sqlite3BtreeSecureDelete(db.Databases[0].pBt,-1) );
  }
  aNew.safety_level = 3;
  aNew.Name = sqlite3DbStrDup(db, Name);
  if( rc==SQLITE_OK && aNew.Name==0 ){
    rc = SQLITE_NOMEM;
  }

  if( rc==SQLITE_OK ){
    extern int sqlite3CodecAttach(sqlite3*, int, const void*, int);
    extern void sqlite3CodecGetKey(sqlite3*, int, void**, int*);
    int nKey;
    char *zKey;
    int t = sqlite3_value_type(argv[2]);
    switch( t ){
      case SQLITE_INTEGER:
      case SQLITE_FLOAT:
        zErrDyn = sqlite3DbStrDup(db, "Invalid key value");
        rc = SQLITE_ERROR;
        break;
        
      case SQLITE_TEXT:
      case SQLITE_BLOB:
        nKey = sqlite3_value_bytes(argv[2]);
        zKey = (char *)sqlite3_value_blob(argv[2]);
        rc = sqlite3CodecAttach(db, len(db.Databases) - 1, zKey, nKey);
        break;

      case SQLITE_NULL:
        /* No key specified.  Use the key from the main database */
        sqlite3CodecGetKey(db, 0, (void**)&zKey, &nKey);
        if( nKey>0 || sqlite3BtreeGetReserve(db.Databases[0].pBt)>0 ){
          rc = sqlite3CodecAttach(db, len(db.Databases) - 1, zKey, nKey);
        }
        break;
    }
  }

  /* If the file was opened successfully, read the schema for the new database.
  ** If this fails, or if opening the file failed, then close the file and 
  ** remove the entry from the db.Databases[] array. i.e. put everything back the way
  ** we found it.
  */
  if( rc==SQLITE_OK ){
    db.LockAll()
    rc = db.Init(zErrDyn)
    db.LeaveBtreeAll()
  }
  if( rc ){
    int iDb = len(db.Databases) - 1
    assert( iDb >= 2 )
    if db.Databases[iDb].pBt != nil {
      sqlite3BtreeClose(db.Databases[iDb].pBt)
      db.Databases[iDb].pBt = nil
      db.Databases[iDb].Schema = nil
    }
    db.ResetInternalSchema(-1)
    db.nDb = iDb;
    if( rc==SQLITE_NOMEM || rc==SQLITE_IOERR_NOMEM ){
      db.mallocFailed = true
      zErrDyn = fmt.Sprintf("out of memory");
    }else if( zErrDyn==0 ){
      zErrDyn = fmt.Sprintf("unable to open database: %v", zFile);
    }
    goto attach_error;
  }
  
  return;

attach_error:
  /* Return an error if we get here */
  if( zErrDyn ){
    sqlite3_result_error(context, zErrDyn, -1);
    zErrDyn = nil
  }
  if( rc ) sqlite3_result_error_code(context, rc);
}

/*
** An SQL user-function registered to do the work of an DETACH statement. The
** three arguments to the function come directly from a detach statement:
**
**     DETACH DATABASE x
**
**     SELECT sqlite_detach(x)
*/
static void detachFunc(
  sqlite3_context *context,
  int NotUsed,
  sqlite3_value **argv
){
  const char *Name = (const char *)sqlite3_value_text(argv[0]);
  sqlite3 *db = sqlite3_context_db_handle(context);
  int i;
  Db *pDb = 0;
  char zErr[128];

  if( Name==0 ) Name = "";
  for i, _ := range db.Databases {
    pDb = &db.Databases[i];
    if( pDb.pBt==0 ) continue;
    if CaseInsensitiveMatch(pDb.Name, Name) {
		break
	}
  }

  if( i>=len(db.Databases) ){
    zErr = fmt.Sprintf("no such database: %s", Name);
    goto detach_error;
  }
  if( i<2 ){
    zErr = fmt.Sprintf("cannot detach database %s", Name);
    goto detach_error;
  }
  if( !db.autoCommit ){
    zErr = "cannot DETACH database within transaction"
    goto detach_error;
  }
  if( sqlite3BtreeIsInReadTrans(pDb.pBt) || sqlite3BtreeIsInBackup(pDb.pBt) ){
    zErr = fmt.Sprintf("database %s is locked", Name);
    goto detach_error;
  }

  sqlite3BtreeClose(pDb.pBt);
  pDb.pBt = nil
  pDb.Schema = nil
  db.ResetInternalSchema(-1)
  return;

detach_error:
  sqlite3_result_error(context, zErr, -1);
}

/*
** This procedure generates VDBE code for a single invocation of either the
** sqlite_detach() or sqlite_attach() SQL user functions.
*/
static void codeAttach(
  Parse *pParse,       /* The parser context */
  int type,            /* Either SQLITE_ATTACH or SQLITE_DETACH */
  FuncDef const *pFunc,/* FuncDef wrapper for detachFunc() or attachFunc() */
  Expr *pAuthArg,      /* Expression to pass to authorization callback */
  Expr *pFilename,     /* Name of database file */
  Expr *pDbname,       /* Name of the database to use internally */
  Expr *pKey           /* Database key for encryption extension */
){
  int rc;
  NameContext sName;
  Vdbe *v;
  sqlite3* db = pParse.db;
  int regArgs;

  memset(&sName, 0, sizeof(NameContext));
  sName.Parse = pParse;

  if( 
      SQLITE_OK!=(rc = resolveAttachExpr(&sName, pFilename)) ||
      SQLITE_OK!=(rc = resolveAttachExpr(&sName, pDbname)) ||
      SQLITE_OK!=(rc = resolveAttachExpr(&sName, pKey))
  ){
    pParse.nErr++;
    goto attach_end;
  }

  if( pAuthArg ){
    char *zAuthArg;
    if( pAuthArg.op==TK_STRING ){
      zAuthArg = pAuthArg.Token;
    }else{
      zAuthArg = 0;
    }
    if pParse.AuthCheck(type, zAuthArg, 0, 0) != SQLITE_OK {
      goto attach_end;
    }
  }

  v = pParse.GetVdbe()
  regArgs = pParse.GetTempRange(4)
  sqlite3ExprCode(pParse, pFilename, regArgs);
  sqlite3ExprCode(pParse, pDbname, regArgs+1);
  sqlite3ExprCode(pParse, pKey, regArgs+2);

  assert( v || db.mallocFailed );
  if( v ){
    v.AddOp3(OP_Function, 0, regArgs+3-pFunc.nArg, regArgs+3);
    assert( pFunc.nArg==-1 || (pFunc.nArg&0xff)==pFunc.nArg );
    v.ChangeP5(byte(pFunc.nArg))
    sqlite3VdbeChangeP4(v, -1, (char *)pFunc, P4_FUNCDEF);

    /* Code an OP_Expire. For an ATTACH statement, set P1 to true (expire this
    ** statement only). For DETACH, set it to false (expire all existing
    ** statements).
    */
    v.AddOp1(OP_Expire, (type==SQLITE_ATTACH));
  }
  
attach_end:
  db.ExprDelete(pFilename)
  db.ExprDelete(pDbname)
  db.ExprDelete(pKey)
}

/*
** Called by the parser to compile a DETACH statement.
**
**     DETACH pDbname
*/
 void sqlite3Detach(Parse *pParse, Expr *pDbname){
  static const FuncDef detach_func = {
    1,                /* nArg */
    SQLITE_UTF8,      /* iPrefEnc */
    0,                /* flags */
    0,                /* pUserData */
    0,                /* Next */
    detachFunc,       /* xFunc */
    0,                /* xStep */
    0,                /* xFinalize */
    "sqlite_detach",  /* Name */
    0,                /* pHash */
    0                 /* pDestructor */
  };
  codeAttach(pParse, SQLITE_DETACH, &detach_func, pDbname, 0, 0, pDbname);
}

/*
** Called by the parser to compile an ATTACH statement.
**
**     ATTACH p AS pDbname KEY pKey
*/
 void sqlite3Attach(Parse *pParse, Expr *p, Expr *pDbname, Expr *pKey){
  static const FuncDef attach_func = {
    3,                /* nArg */
    SQLITE_UTF8,      /* iPrefEnc */
    0,                /* flags */
    0,                /* pUserData */
    0,                /* Next */
    attachFunc,       /* xFunc */
    0,                /* xStep */
    0,                /* xFinalize */
    "sqlite_attach",  /* Name */
    0,                /* pHash */
    0                 /* pDestructor */
  };
  codeAttach(pParse, SQLITE_ATTACH, &attach_func, p, p, pDbname, pKey);
}
#endif /* SQLITE_OMIT_ATTACH */