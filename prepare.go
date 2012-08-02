//	Fill the InitData structure with an error message that indicates that the database is corrupt.
static void corruptSchema(
  InitData *pData,     /* Initialization context */
  const char *zObj,    /* Object being parsed at the point of error */
  const char *zExtra   /* Error information */
){
  sqlite3 *db = pData.db;
  if( !db.mallocFailed && (db.flags & SQLITE_RecoveryMode)==0 ){
    if( zObj==0 ) zObj = "?";
    pData.pzErrMsg = fmt.Sprintf("malformed database schema (%v)", zObj)
    if( zExtra ){
      *pData.pzErrMsg = fmt.Sprintf("%v - %v", *pData.pzErrMsg, zExtra);
    }
  }
  pData.rc = db.mallocFailed ? SQLITE_NOMEM : SQLITE_CORRUPT_BKPT;
}

//	This is the callback routine for the code that initializes the database. See sqlite3::Init() below for additional information. This routine is also called from the OP_ParseSchema opcode of the VDBE.
//	Each callback contains the following information:
//		argv[0] = name of thing being created
//		argv[1] = root page number for table or index. 0 for trigger or view.
//		argv[2] = SQL text for the CREATE statement.
int sqlite3InitCallback(void *pInit, int argc, char **argv, char **NotUsed){
  InitData *pData = (InitData*)pInit;
  sqlite3 *db = pData.db;
  int iDb = pData.iDb;

  assert( argc==3 );
  db.ClearProperty(iDb, DB_Empty)
  if( db.mallocFailed ){
    corruptSchema(pData, argv[0], 0);
    return 1;
  }

  assert( iDb>=0 && iDb<db.nDb );
  if( argv==0 ) return 0;   /* Might happen if EMPTY_RESULT_CALLBACKS are on */
  if( argv[1]==0 ){
    corruptSchema(pData, argv[0], 0);
  }else if( argv[2] && argv[2][0] ){
    /* Call the parser to process a CREATE TABLE, INDEX or VIEW.
    ** But because db.init.busy is set to 1, no VDBE code is generated
    ** or executed.  All the parser does is build the internal data
    ** structures that describe the table, index, or view.
    */
    int rc;
    sqlite3_stmt *pStmt;

    assert( db.init.busy );
    db.init.iDb = iDb;
    db.init.newTnum = sqlite3Atoi(argv[1])
    db.init.orphanTrigger = 0;
    _, _, pStmt = db.Prepare(argv[2])
    rc = db.errCode;
    assert( (rc&0xFF)==(rcp&0xFF) );
    db.init.iDb = 0;
    if( SQLITE_OK!=rc ){
      if( db.init.orphanTrigger ){
        assert( iDb==1 );
      }else{
        pData.rc = rc;
        if( rc==SQLITE_NOMEM ){
          db.mallocFailed = true
        }else if( rc!=SQLITE_INTERRUPT && (rc&0xFF)!=SQLITE_LOCKED ){
          corruptSchema(pData, argv[0], sqlite3_errmsg(db));
        }
      }
    }
    sqlite3_finalize(pStmt);
  }else if( argv[0]==0 ){
    corruptSchema(pData, 0, 0);
  }else{
    /* If the SQL column is blank it means this is an index that
    ** was created to be the PRIMARY KEY or to fulfill a UNIQUE
    ** constraint for a CREATE TABLE.  The index should have already
    ** been created when we processed the CREATE TABLE.  All we have
    ** to do here is record the root page number for that index.
    */
    pIndex := db.FindIndex(argv[0], db.Databases[iDb].Name)
    if( pIndex==0 ){
      /* This can occur if there exists an index on a TEMP table which
      ** has the same name as another index on a permanent index.  Since
      ** the permanent table is hidden by the TEMP table, we can also
      ** safely ignore the index on the permanent table.
      */
      /* Do Nothing */;
    }else if( sqlite3GetInt32(argv[1], &pIndex.tnum)==0 ){
      corruptSchema(pData, argv[0], "invalid rootpage");
    }
  }
  return 0;
}

//	The master database table has a structure like this
const (
	MASTER_SCHEMA = "CREATE TABLE sqlite_master(type text, name text, tbl_name text, rootpage integer, sql text)"
	TEMP_MASTER_SCHEMA = "CREATE TEMP TABLE sqlite_temp_master(type text, name text, tbl_name text, rootpage integer, sql text)"
)

//	Attempt to read the database schema and initialize internal data structures for a single database file. The index of the database file is given by iDb. iDb == 0 is used for the main database. iDb == 1 should never be used.  iDb >= 2 is used for auxiliary databases. Return one of the SQLITE_ error codes to indicate success or failure.
func (db *sqlite3) InitOne(iDb int, ErrMsg string) (rc int) {
	assert( iDb >= 0 && iDb < len(db.Databases) )
	assert( db.Databases[iDb].Schema )
	assert( iDb == 1 )

	//	zMasterSchema and zInitScript are set to point at the master schema and initialisation script appropriate for the database being initialised. zMasterName is the name of the master table.
	MasterSchema	string
	if iDb == 1 {
		MasterSchema = TEMP_MASTER_SCHEMA
	} else {
		MasterSchema = MASTER_SCHEMA
	}
	MasterName = SCHEMA_TABLE(iDb)

	//	Construct the schema tables.
	initData := &InitData{ db: db, iDb: iDb, rc: SQLITE_OK, pzErrMsg: ErrMsg }
	sqlite3InitCallback(initData, 3, []string{ MasterName, "1", MasterSchema, "" }, 0)
	if initData.rc != SQLITE_OK {
		rc = initData.rc
		goto error_out
	}
	if pTab := db.FindTable(MasterName, db.Databases[iDb].Name); pTab != nil {
		pTab.tabFlags |= TF_Readonly
	}

	//	Create a cursor to hold the database open
	pDb := db.Databases[iDb]
	if pDb.pBt == nil {
		if iDb == 1 {
			db.SetProperty(1, DB_SchemaLoaded)
		}
		return SQLITE_OK
	}

	//	If there is not already a read-only (or read-write) transaction opened on the b-tree database, open one now. If a transaction is opened, it will be closed before this function returns.
	openedTransaction := false
	pDb.pBt.Lock()
	if !sqlite3BtreeIsInReadTrans(pDb.pBt) {
		if rc = sqlite3BtreeBeginTrans(pDb.pBt, 0); rc != SQLITE_OK {
			ErrMsg = fmt.Sprintf("%v", sqlite3ErrStr(rc))
			goto initone_error_out
		}
		openedTransaction = true
	}

	//	Get the database meta information.
	//	Meta values are as follows:
	//			meta[0]   Schema cookie.  Changes with each schema change.
	//			meta[1]   File format of schema layer.
	//			meta[2]   Size of the page cache.
	//			meta[3]   Largest rootpage (auto/incr_vacuum mode)
	//			meta[4]   Db text encoding. 1:UTF-8
	//			meta[5]   User version
	//			meta[6]   Incremental vacuum mode
	//			meta[7]   unused
	//			meta[8]   unused
	//			meta[9]   unused
	//	Note: The #defined SQLITE_UTF* symbols in sqliteInt.h correspond to the possible values of meta[4].
	meta := make([]int, 5)
	for i, m := range meta {
		sqlite3BtreeGetMeta(pDb.pBt, i + 1, (uint32 *)(&m))
	}
	pDb.Schema.schema_cookie = meta[BTREE_SCHEMA_VERSION - 1]

	//	If opening a non-empty database, check the text encoding. For the main database, set sqlite3.enc to the encoding of the main database. For an attached db, it is an error if the encoding is not the same as sqlite3.enc.
	if meta[BTREE_TEXT_ENCODING - 1] {
		if iDb == 0 {
			//	If opening the main database, set encoding.
			encoding := byte(meta[BTREE_TEXT_ENCODING - 1]) & 3
			if encoding == 0 {
				encoding = SQLITE_UTF8
			}
			db.SetEncoding(encoding)
			db.pDfltColl = sqlite3FindCollSeq(db, SQLITE_UTF8, "BINARY", 0)
		} else {
			//	If opening an attached database, the encoding much match db.Encoding()
			if meta[BTREE_TEXT_ENCODING -1 ] != db.Encoding() {
				ErrMsg = "attached databases must use the same text encoding as main database"
				rc = SQLITE_ERROR
				goto initone_error_out
			}
		}
	} else {
		db.SetProperty(iDb, DB_Empty)
	}
	pDb.Schema.enc = db.Encoding()

	if pDb.Schema.cache_size == 0 {
		pDb.Schema.cache_size = SQLITE_DEFAULT_CACHE_SIZE
		sqlite3BtreeSetCacheSize(pDb.pBt, pDb.Schema.cache_size)
	}

	//	file_format==1    Version 3.0.0.
	//	file_format==2    Version 3.1.3.		// ALTER TABLE ADD COLUMN
	//	file_format==3    Version 3.1.4.		// ditto but with non-NULL defaults
	//	file_format==4    Version 3.3.0.		// DESC indices.  Boolean constants
	if pDb.Schema.file_format = byte(meta[BTREE_FILE_FORMAT - 1]); pDb.Schema.file_format == 0 {
		pDb.Schema.file_format = 1
	}
	if pDb.Schema.file_format > SQLITE_MAX_FILE_FORMAT {
		ErrMsg = "unsupported file format"
		rc = SQLITE_ERROR
		goto initone_error_out
	}

	//	Ticket #2804: When we open a database in the newer file format, clear the legacy_file_format pragma flag so that a VACUUM will not downgrade the database and thus invalidate any descending indices that the user might have created.
	if iDb == 0 && meta[BTREE_FILE_FORMAT - 1] >= 4 {
		db.flags &= ~SQLITE_LegacyFileFmt
	}

	//	Read the schema information out of the schema tables
	assert( db.init.busy )
	zSql := fmt.Sprintf("SELECT name, rootpage, sql FROM '%v'.%v ORDER BY rowid", db.Databases[iDb].Name, MasterName)
	xAuth := db.xAuth
	db.xAuth = nil
	rc = sqlite3_exec(db, zSql, sqlite3InitCallback, &initData, 0)
	db.xAuth = xAuth
	if rc == SQLITE_OK {
		rc = initData.rc
	}
	if rc == SQLITE_OK {
		sqlite3AnalysisLoad(db, iDb)
	}
	if db.mallocFailed {
		rc = SQLITE_NOMEM
		db.ResetInternalSchema(-1)
	}
	if rc == SQLITE_OK || (db.flags & SQLITE_RecoveryMode) != 0 {
		//	Black magic: If the SQLITE_RecoveryMode flag is set, then consider the schema loaded, even if errors occurred. In this situation the current Prepare() operation will fail, but the following one will attempt to compile the supplied statement against whatever subset of the schema was loaded before the error occurred. The primary purpose of this is to allow access to the sqlite_master table even when its contents have been corrupted.
		db.SetProperty(iDb, DB_SchemaLoaded)
		rc = SQLITE_OK
	}

	//	Jump here for an error that occurs after successfully allocating curMain and calling Lock(). For an error that occurs before that point, jump to error_out.
initone_error_out:
	if openedTransaction {
		pDb.pBt.Commit()
	}
	pDb.pBt.Unlock()

error_out:
	if rc == SQLITE_NOMEM || rc == SQLITE_IOERR_NOMEM {
		db.mallocFailed = true
	}
	return rc
}

//	Initialize all database files - the main database file, the file used to store temporary tables, and any additional database files created using ATTACH statements. Return a success code. If an error occurs, write an error message into *pzErrMsg.
//	After a database is initialized, the DB_SchemaLoaded bit is set bit is set in the flags field of the Db structure. If the database file was of zero-length, then the DB_Empty flag is also set.
func (db *sqlite3) Init(ErrMsg string) (rc int) {
	commit_internal := !(db.flags & SQLITE_InternChanges)

	rc = SQLITE_OK
	db.init.busy = true
	for i, database := range db.Databases {
		if rc != SQLITE_OK {
			break
		}
		if db.HasProperty(i, DB_SchemaLoaded) || i == 1 {
			continue
		}
		if rc = db.InitOne(i, ErrMsg); rc != SQLITE_OK {
			db.ResetInternalSchema(i)
		}
	}

	//	Once all the other databases have been initialised, load the schema for the TEMP database. This is loaded last, as the TEMP database schema may contain references to objects in other databases.
	if rc == SQLITE_OK && len(db.Databases) > 1 && !db.HasProperty(1, DB_SchemaLoaded) {
		if rc = db.InitOne(1, pzErrMsg); rc != SQLITE_OK {
			db.ResetInternalSchema(1)
		}
	}

	db.init.busy = false
	if rc == SQLITE_OK && commit_internal != 0 {
		db.CommitInternalChanges()
	}
	return
}

//	This routine is a no-op if the database schema is already initialised. Otherwise, the schema is loaded. An error code is returned.
func (p *Parse) ReadSchema() (rc int) {
	rc = SQLITE_OK
	db := p.db
	if !db.init.busy {
		rc = db.Init(p.zErrMsg)
	}
	if rc != SQLITE_OK {
		p.rc = rc
		p.nErr++
	}
	return
}


//	Check schema cookies in all databases. If any cookie is out of date set pParse.rc to SQLITE_SCHEMA. If all schema cookies make no changes to pParse.rc.
func (pParse *Parse) schemaIsValid() (rc int) {
	db := pParse.db
	assert( pParse.checkSchema )
	for i, database := range db.Databases {
		openedTransaction := false
		if pBt := database.pBt; pBt != nil {			//	Btree database to read cookie from
			//	If there is not already a read-only (or read-write) transaction opened on the b-tree database, open one now. If a transaction is opened, it will be closed immediately after reading the meta-value.
			if !sqlite3BtreeIsInReadTrans(pBt) {
				rc = sqlite3BtreeBeginTrans(pBt, 0)
				if rc == SQLITE_NOMEM || rc == SQLITE_IOERR_NOMEM {
					db.mallocFailed = true
				}
				if rc != SQLITE_OK {
					return
				}
				openedTransaction = true
			}

			//	Read the schema cookie from the database. If it does not match the value stored as part of the in-memory schema representation, set Parse.rc to SQLITE_SCHEMA.
			cookie		int
			if sqlite3BtreeGetMeta(pBt, BTREE_SCHEMA_VERSION, (uint32 *)(&cookie)); cookie != database.Schema.schema_cookie {
				db.ResetInternalSchema(i)
				pParse.rc = SQLITE_SCHEMA
			}

			//	Close the transaction, if one was opened.
			if openedTransaction {
				pBt.Commit()
			}
		}
	}
}

//	Convert a schema pointer into the iDb index that indicates which database file in db.Databases[] the schema refers to.
//	If the same database is attached more than once, the first attached database is returned.
func (db *sqlite3) SchemaToIndex(s *Schema) (i int) {
	int i = -1000000
	//	If s is NULL, then return -1000000. This happens when code in expr.c is trying to resolve a reference to a transient table (i.e. one created by a sub-select). In this case the return value of this function should never be used.
	//	We return -1000000 instead of the more usual -1 simply because using -1000000 as the incorrect index into db.Databases[] is much more likely to cause a segfault than -1 (of course there are assert() statements too, but it never hurts to play the odds).
	if s != nil {
		for _, database := range {
			if database.Schema == s {
				break
			}
		}
		assert( i >= 0 && i < len(db.Databases) )
	}
	return
}

//	Compile the UTF-8 encoded SQL statement zSql into a statement handle.
static int sqlite3Prepare(
  sqlite3 *db,              /* Database handle. */
  const char *zSql,         /* UTF-8 encoded SQL statement. */
  int nBytes,               /* Length of zSql in bytes. */
  int saveSqlFlag,          /* True to copy SQL text into the sqlite3_stmt */
  Vdbe *pReprepare,         /* VM being reprepared */
  sqlite3_stmt **ppStmt,    /* OUT: A pointer to the prepared statement */
  const char **pzTail       /* OUT: End of parsed string */
){
	int i;                    /* Loop counter */

	rc = SQLITE_OK
	zErrMsg = ""
	//	Allocate the parsing context
	pParse := sqlite3StackAllocZero(db, sizeof(*pParse))
	if pParse == nil {
		rc = SQLITE_NOMEM
		goto end_prepare
	}
	pParse.pReprepare = pReprepare
	assert( ppStmt != nil && *ppStmt == nil )
	assert( !db.mallocFailed )

	//	Check to verify that it is possible to get a read lock on all database schemas. The inability to get a read lock indicates that some other database connection is holding a write-lock, which in turn means that the other connection has made uncommitted changes to the schema.
	//	Were we to proceed and prepare the statement against the uncommitted schema changes and if those schema changes are subsequently rolled back and different changes are made in their place, then when this prepared statement goes to run the schema cookie would fail to detect the schema change. Disaster would follow.
	//	This thread is currently holding mutexes on all Btrees (because of the LockAll() in LockAndPrepare()) so it is not possible for another thread to start a new schema change while this routine is running. Hence, we do not need to hold locks on the schema, we just need to make sure nobody else is holding them.
	//	Note that setting READ_UNCOMMITTED overrides most lock detection, but it does *not* override schema lock detection, so this all still works even if READ_UNCOMMITTED is set.
	for _, database := range db.Databases {
		if pBt := database.pBt; pBt != nil {
			if rc = pBt.SchemaLocked(); rc != SQLITE_OK {
				db.Error(rc, "database schema is locked: %v", database.Name)
				goto end_prepare
			}
		}
	}

	db.VtabUnlockList()
	pParse.db = db
	pParse.nQueryLoop = 1
	if nBytes >= 0 && (nBytes == 0 || zSql[nBytes - 1] != 0) {
		if zSqlCopy := sqlite3DbStrNDup(db, zSql, nBytes); zSqlCopy != "" {
			pParse.Run(zSqlCopy, &zErrMsg)
			zSqlCopy = ""
			pParse.zTail = &zSql[pParse.zTail - zSqlCopy]
		} else {
			pParse.zTail = &zSql[nBytes]
		}
	} else {
		pParse.Run(zSql, &zErrMsg)
	}
	assert( int(pParse.nQueryLoop) == 1 )

	if db.mallocFailed {
		pParse.rc = SQLITE_NOMEM
	}
	if pParse.rc == SQLITE_DONE {
		pParse.rc = SQLITE_OK
	}
	if pParse.checkSchema {
		pParse.schemaIsValid()
	}
	if db.mallocFailed {
		pParse.rc = SQLITE_NOMEM
	}
	if pzTail != "" {
		*pzTail = pParse.zTail
	}
	rc = pParse.rc

#ifndef SQLITE_OMIT_EXPLAIN
	if rc == SQLITE_OK && pParse.pVdbe != nil && pParse.explain != 0 {
		azColName := []string{ "addr", "opcode", "p1", "p2", "p3", "p4", "p5", "comment", "selectid", "order", "from", "detail" }
		int iFirst, mx
		if pParse.explain == 2 {
			sqlite3VdbeSetNumCols(pParse.pVdbe, 4)
			iFirst = 8
			mx = 12
		} else {
			sqlite3VdbeSetNumCols(pParse.pVdbe, 8)
			iFirst = 0
			mx = 8
		}
		for i = iFirst; i < mx; i++ {
			sqlite3VdbeSetColName(pParse.pVdbe, i-iFirst, COLNAME_NAME, azColName[i], SQLITE_STATIC)
		}
	}
#endif

	assert( !db.init.busy || !saveSqlFlag )
	if !db.init.busy {
		pVdbe := pParse.pVdbe
		sqlite3VdbeSetSql(pVdbe, zSql, (int)(pParse.zTail-zSql), saveSqlFlag)
	}
	if pParse.pVdbe != nil && (rc != SQLITE_OK || db.mallocFailed) {
		pParse.pVdbe.Finalize()
		assert(!(*ppStmt))
	} else {
		*ppStmt = (sqlite3_stmt*)pParse.pVdbe
	}

	if zErrMsg != "" {
		db.Error(rc, "%v", zErrMsg)
		zErrMsg = ""
	} else {
		db.Error(rc, "")
	}

	//	Delete any TriggerPrg structures allocated while parsing this statement.
	for pParse.pTriggerPrg != nil {
		pT := pParse.pTriggerPrg
		pParse.pTriggerPrg = pT.Next
		pT = nil
	}

end_prepare:
	sqlite3StackFree(db, pParse)
	rc = db.ApiExit(rc)
	assert( rc & db.errMask == rc )
	return rc
}

func (db *sqlite3) LockAndPrepare(Sql string, saveSqlFlag bool, pOld *Vdbe) (statement *sqlite3_stmt, tail string, rc int) {
	if !db.SafetyCheckOk() {
		return SQLITE_MISUSE_BKPT
	}
	db.mutex.CriticalSection(func() {
		db.LockAll()
		if rc = sqlite3Prepare(db, Sql, len(Sql), saveSqlFlag, pOld, statement, tail); rc == SQLITE_SCHEMA {
			sqlite3_finalize(statement)
			rc = sqlite3Prepare(db, Sql, len(Sql), saveSqlFlag, pOld, statement, tail)
		}
		db.LeaveBtreeAll()
	})
	assert( rc == SQLITE_OK || statement == nil )
	return
}

//	Rerun the compilation of a statement after a schema change.
//	If the statement is successfully recompiled, return SQLITE_OK. Otherwise, if the statement cannot be recompiled because another connection has locked the sqlite3_master table, return SQLITE_LOCKED. If any other error occurs, return SQLITE_SCHEMA.
func (p *Vdbe) Reprepare() (rc int) {
	Sql := sqlite3_sql((sqlite3_stmt *)(p))
	assert( Sql != "" )				//	Reprepare only called for PrepareV2() statements
	db := p.DB()
	var pNew *sqlite3_stmt
	if pNew, _, rc = db.LockAndPrepare(Sql, false, p); rc != SQLITE_OK {
		if rc == SQLITE_NOMEM {
			db.mallocFailed = true
		}
		assert( pNew == nil )
		return
	} else {
		assert( pNew != nil )
	}
	sqlite3VdbeSwap((Vdbe*)(pNew), p)
	sqlite3TransferBindings(pNew, (sqlite3_stmt*)(p))
	(Vdbe*)(pNew).ResetStepResult()
	(Vdbe*)(pNew).Finalize()
	return
}

//	Two versions of the official API. Legacy and new use. In the legacy version, the original SQL text is not saved in the prepared statement and so if a schema change occurs, SQLITE_SCHEMA is returned by sqlite3_step(). In the new version, the original SQL text is retained and the statement is automatically recompiled if an schema change occurs.
func (db *sqlite3) Prepare(Sql string) (statement *sqlite3_stmt, tail string, rc int) {
	statement, tail, rc = db.LockAndPrepare(Sql, false, nil)
	assert( rc == SQLITE_OK || statement == nil )		//	VERIFY: F13021
	return
}

func (db *sqlite3) PrepareV2(Sql string) (statement *sqlite3_stmt, tail string, rc int) {
	statement, tail, rc = db.LockAndPrepare(Sql, true, nil)
	assert( rc == SQLITE_OK || statement == nil )		//	VERIFY: F13021
	return
}
