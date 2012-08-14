/* This file contains code used to implement the VACUUM command.
**
** Most of the code in this file may be omitted by defining the
** SQLITE_OMIT_VACUUM macro.
*/

#if !defined(SQLITE_OMIT_VACUUM) && !defined(SQLITE_OMIT_ATTACH)
//	Finalize a prepared statement. If there was an error, store the text of the error message in *pzErrMsg. Return the result code.
func (db *sqlite3) vacuumFinalize(pStmt *sqlite3_stmt) (errMsg, rc int) {
	if rc = (Vdbe*)(pStmt).Finalize(); rc != SQLITE_OK {
		errMsg = sqlite3_errmsg(db)
	}
	return
}

//	Execute zSql on database db. Return an error code.
func (db *sqlite3) ExecSql(Sql string) (errMsg string, rc int) {
	var pStmt	*sqlite3_stmt
	if pStmt, _, rc = db.Prepare(Sql, -1) != SQLITE_OK {
		return sqlite3_errmsg(db), sqlite3_errcode(db)
	}
	sqlite3_step(pStmt)
	assert( rc != SQLITE_ROW || db.flags & SQLITE_CountRows )
	return db.vacuumFinalize(pStmt)
}

//	Execute zSql on database db. The statement returns exactly one column. Execute this as SQL on the same database.
func (db *sqlite3) execExecSql(Sql string) (errMsg string, rc int) {
	var pStmt	*sqlite3_stmt

	if pStmt, _, rc = db.Prepare(Sql); rc != SQLITE_OK {
		return
	}

	for SQLITE_ROW == sqlite3_step(pStmt) {
		if errMsg, rc = db.ExecSql(string(sqlite3_column_text(pStmt, 0))); rc != SQLITE_OK {
			return db.vacuumFinalize(pStmt)
		}
	}
	return db.vacuumFinalize(pStmt)
}

//	The non-standard VACUUM command is used to clean up the database, collapse free space, etc. It is modelled after the VACUUM command in PostgreSQL.
//	In version 1.0.x of SQLite, the VACUUM command would call gdbm_reorganize() on all the database tables. But beginning with 2.0.0, SQLite no longer uses GDBM so this command has become a no-op.
func (pParse *Parse) Vacuum() {
	if v := pParse.GetVdbe(); v != nil {
		v.AddOp2(OP_Vacuum, 0, 0)
	}
}

//	This routine implements the OP_Vacuum opcode of the VDBE.
int sqlite3RunVacuum(char **pzErrMsg, sqlite3 *db){
  int rc = SQLITE_OK;     /* Return code from service routines */
  Btree *pMain;           /* The database being vacuumed */
  Btree *pTemp;           /* The temporary database we vacuum into */
  char *zSql = 0;         /* SQL statements */
  int saved_flags;        /* Saved value of the db.flags */
  int saved_nChange;      /* Saved value of db.nChange */
  int saved_nTotalChange; /* Saved value of db.nTotalChange */
  void (*saved_xTrace)(void*,const char*);  /* Saved db.xTrace */
  Db *pDb = 0;            /* Database to detach at end of vacuum */
  int isMemDb;            /* True if vacuuming a :memory: database */
  int nRes;               /* Bytes of reserved space at the end of each page */
  int nDb;                /* Number of attached databases */

  if( !db.autoCommit ){
    pzErrMsg = "cannot VACUUM from within a transaction"
    return SQLITE_ERROR;
  }
  if( db.activeVdbeCnt>1 ){
    pzErrMsg = "cannot VACUUM - SQL statements in progress"
    return SQLITE_ERROR;
  }

  //	Save the current value of the database flags so that it can be restored before returning. Then set the writable-schema flag, and disable CHECK and foreign key constraints.
  saved_flags = db.flags;
  saved_nChange = db.nChange;
  saved_nTotalChange = db.nTotalChange;
  saved_xTrace = db.xTrace;
  db.flags |= SQLITE_WriteSchema | SQLITE_IgnoreChecks | SQLITE_PreferBuiltin;
  db.flags &= ~(SQLITE_ForeignKeys | SQLITE_ReverseOrder);
  db.xTrace = 0;

  pMain = db.Databases[0].pBt;
  isMemDb = sqlite3PagerIsMemdb(pMain.Pager());

  //	Attach the temporary database as 'vacuum_db'. The synchronous pragma can be set to 'off' for this file, as it is not recovered if a crash occurs anyway. The integrity of the database is maintained by a (possibly synchronous) transaction opened on the main database before sqlite3BtreeCopyFile() is called.
  //	An optimisation would be to use a non-journaled pager.
  //	(Later:) I tried setting "PRAGMA vacuum_db.journal_mode=OFF" but that actually made the VACUUM run slower. Very little journalling actually occurs when doing a vacuum since the vacuum_db is initially empty. Only the journal header is written. Apparently it takes more time to parse and run the PRAGMA to turn journalling off than it does to write the journal header file.
  nDb = len(db.Databases)
  if( sqlite3TempInMemory(db) ){
    zSql = "ATTACH ':memory:' AS vacuum_db;";
  }else{
    zSql = "ATTACH '' AS vacuum_db;";
  }
  pzErrMsg, rc = db.ExecSql(zSql)
  if len(db.Databases) > nDb {
    pDb = &db.Databases[len(db.Databases) - 1]
    assert( strcmp(pDb.Name,"vacuum_db") == 0 )
  }
  if rc != SQLITE_OK {
	  goto end_of_vacuum
	}
  pTemp = db.Databases[len(db.Databases) - 1].pBt

  //	The call to ExecSql() to attach the temp database has left the file locked (as there was more than one active statement when the transaction to read the schema was concluded. Unlock it here so that this doesn't cause problems for the call to BtreeSetPageSize() below.
  pTemp.Commit()

  nRes = sqlite3BtreeGetReserve(pMain);

  //	A VACUUM cannot change the pagesize of an encrypted database.
  if db.nextPagesize != 0 {
    extern void sqlite3CodecGetKey(sqlite3*, int, void**, int*);
    int nKey;
    char *zKey;
    sqlite3CodecGetKey(db, 0, (void**)&zKey, &nKey);
    if nKey {
		db.nextPagesize = 0
	}
  }

  if pzErrMsg, rc = db.ExecSql("PRAGMA vacuum_db.synchronous=OFF"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }

  //	Begin a transaction and take an exclusive lock on the main database file. This is done before the sqlite3BtreeGetPageSize(pMain) call below, to ensure that we do not try to change the page-size on a WAL database.
  if pzErrMsg, rc = db.ExecSql("BEGIN;"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }
  if rc = pMain.BeginTransaction(2); rc != SQLITE_OK {
	  goto end_of_vacuum
  }

  //	Do not attempt to change the page size for a WAL database
  if sqlite3PagerGetJournalMode(pMain.Pager()) == PAGER_JOURNALMODE_WAL {
    db.nextPagesize = 0
  }

  if( sqlite3BtreeSetPageSize(pTemp, sqlite3BtreeGetPageSize(pMain), nRes, 0)
   || (!isMemDb && sqlite3BtreeSetPageSize(pTemp, db.nextPagesize, nRes, 0))
   || db.mallocFailed
  ){
    rc = SQLITE_NOMEM;
    goto end_of_vacuum;
  }

  sqlite3BtreeSetAutoVacuum(pTemp, db.nextAutovac>=0 ? db.nextAutovac :
                                           sqlite3BtreeGetAutoVacuum(pMain));

  //	Query the schema of the main database. Create a mirror schema in the temporary database.
  if pzErrMsg, rc = db.execExecSql("SELECT 'CREATE TABLE vacuum_db.' || substr(sql,14) FROM sqlite_master WHERE type='table' AND name!='sqlite_sequence' AND rootpage>0"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }
  if pzErrMsg, rc = db.execExecSql("SELECT 'CREATE INDEX vacuum_db.' || substr(sql,14) FROM sqlite_master WHERE sql LIKE 'CREATE INDEX %' "); rc != SQLITE_OK {
	  goto end_of_vacuum
  }
  if pzErrMsg, rc = db.execExecSql("SELECT 'CREATE UNIQUE INDEX vacuum_db.' || substr(sql,21) FROM sqlite_master WHERE sql LIKE 'CREATE UNIQUE INDEX %'"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }
  //	Loop through the tables in the main database. For each, do an "INSERT INTO vacuum_db.xxx SELECT * FROM main.xxx;" to copy the contents to the temporary database.
  if pzErrMsg, rc = db.execExecSql("SELECT 'INSERT INTO vacuum_db.' || quote(name) || ' SELECT * FROM main.' || quote(name) || ';' FROM main.sqlite_master WHERE type = 'table' AND name!='sqlite_sequence' AND rootpage>0"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }

  //	Copy over the sequence table
  if pzErrMsg, rc = db.execExecSql("SELECT 'DELETE FROM vacuum_db.' || quote(name) || ';' FROM vacuum_db.sqlite_master WHERE name='sqlite_sequence' "); rc != SQLITE_OK {
	  goto end_of_vacuum
  }
  if pzErrMsg, rc = db.execExecSql("SELECT 'INSERT INTO vacuum_db.' || quote(name) || ' SELECT * FROM main.' || quote(name) || ';' FROM vacuum_db.sqlite_master WHERE name=='sqlite_sequence';"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }

  //	Copy the triggers, views, and virtual tables from the main database over to the temporary database. None of these objects has any associated storage, so all we have to do is copy their entries from the SQLITE_MASTER table.
  if pzErrMsg, rc = db.ExecSql("INSERT INTO vacuum_db.sqlite_master SELECT type, name, tbl_name, rootpage, sql FROM main.sqlite_master WHERE type='view' OR type='trigger' OR (type='table' AND rootpage=0)"); rc != SQLITE_OK {
	  goto end_of_vacuum
  }

  //	At this point, there is a write transaction open on both the vacuum database and the main database. Assuming no error occurs, both transactions are closed by this block - the main database transaction by sqlite3BtreeCopyFile() and the other by an explicit call to Btree::Commit().
  {
    uint32 meta;
    int i;

    /* This array determines which meta meta values are preserved in the
    ** vacuum.  Even entries are the meta value number and odd entries
    ** are an increment to apply to the meta value after the vacuum.
    ** The increment is used to increase the schema cookie so that other
    ** connections to the same database will know to reread the schema.
    */
    static const unsigned char aCopy[] = {
       BTREE_SCHEMA_VERSION,     1,  /* Add one to the old schema cookie */
       BTREE_DEFAULT_CACHE_SIZE, 0,  /* Preserve the default page cache size */
       BTREE_TEXT_ENCODING,      0,  /* Preserve the text encoding */
       BTREE_USER_VERSION,       0,  /* Preserve the user version */
    };

    assert( pTemp.IsInTrans() )
    assert( pMain.IsInTrans() )

    /* Copy Btree meta values */
    for(i=0; i<ArraySize(aCopy); i+=2){
      /* GetMeta() and UpdateMeta() cannot fail in this context because
      ** we already have page 1 loaded into cache and marked dirty. */
      sqlite3BtreeGetMeta(pMain, aCopy[i], &meta);
      if rc = sqlite3BtreeUpdateMeta(pTemp, aCopy[i], meta+aCopy[i+1]); rc != SQLITE_OK {
		  goto end_of_vacuum
	  }
    }

	if rc = sqlite3BtreeCopyFile(pMain, pTemp); rc != SQLITE_OK {
		goto end_of_vacuum
	}
    if rc = pTemp.Commit(); rc != SQLITE_OK {
		goto end_of_vacuum
	}
    sqlite3BtreeSetAutoVacuum(pMain, sqlite3BtreeGetAutoVacuum(pTemp));
  }

  assert( rc==SQLITE_OK );
  rc = sqlite3BtreeSetPageSize(pMain, sqlite3BtreeGetPageSize(pTemp), nRes,1);

end_of_vacuum:
	//	Restore the original value of db.flags */
	db.flags = saved_flags
	db.nChange = saved_nChange
	db.nTotalChange = saved_nTotalChange
	db.xTrace = saved_xTrace
	sqlite3BtreeSetPageSize(pMain, -1, -1, 1)

	//	Currently there is an SQL level transaction open on the vacuum database. No locks are held on any other files (since the main file was committed at the btree level). So it safe to end the transaction by manually setting the autoCommit flag to true and detaching the vacuum database. The vacuum_db journal file is deleted when the pager is closed by the DETACH.
	db.autoCommit = 1

	if pDb != nil {
		sqlite3BtreeClose(pDb.pBt)
		pDb.pBt = nil
		pDb.Schema = nil
	}

	//	This both clears the schemas and reduces the size of the db.Databases[] array.
	db.ResetInternalSchema(-1)
	return rc
}

#endif  /* SQLITE_OMIT_VACUUM && SQLITE_OMIT_ATTACH */
