/* This file contains code used to implement the PRAGMA command.
*/

/*
** Interpret the given string as a safety level.  Return 0 for OFF,
** 1 for ON or NORMAL and 2 for FULL.  Return 1 for an empty or 
** unrecognized string argument.  The FULL option is disallowed
** if the omitFull parameter it 1.
**
** Note that the values returned are one less that the values that
** should be passed into sqlite3BtreeSetSafetyLevel().  The is done
** to support legacy SQL code.  The safety level used to be boolean
** and older scripts may have used numbers 0 for OFF and 1 for ON.
*/
static byte getSafetyLevel(const char *z, int omitFull, int dflt){
                             /* 123456789 123456789 */
  static const char zText[] = "onoffalseyestruefull";
  static const byte iOffset[] = {0, 1, 2, 4, 9, 12, 16};
  static const byte iLength[] = {2, 2, 3, 5, 3, 4, 4};
  static const byte Value[] =  {1, 0, 0, 0, 1, 1, 2};
  int i, n;
  if( sqlite3Isdigit(*z) ){
    return (byte)sqlite3Atoi(z);
  }
  n = sqlite3Strlen30(z);
  for(i=0; i<ArraySize(iLength)-omitFull; i++){
    if iLength[i] == n && CaseInsensitiveMatchN(&zText[iOffset[i]],z,n) {
      return Value[i];
    }
  }
  return dflt;
}

/*
** Interpret the given string as a boolean value.
*/
 byte sqlite3GetBoolean(const char *z, int dflt){
  return getSafetyLevel(z,1,dflt)!=0;
}

/* The sqlite3GetBoolean() function is used by other modules but the
** remainder of this file is specific to PRAGMA processing.  So omit
** the rest of the file if PRAGMAs are omitted from the build.
*/
#if !defined(SQLITE_OMIT_PRAGMA)

/*
** Interpret the given string as a locking mode value.
*/
static int getLockingMode(const char *z){
  if( z ){
    if CaseInsensitiveMatch(z, "exclusive") {
		return PAGER_LOCKINGMODE_EXCLUSIVE
	}
    if CaseInsensitiveMatch(z, "normal") {
		return PAGER_LOCKINGMODE_NORMAL
	}
  }
  return PAGER_LOCKINGMODE_QUERY;
}

/*
** Interpret the given string as an auto-vacuum mode value.
**
** The following strings, "none", "full" and "incremental" are 
** acceptable, as are their numeric equivalents: 0, 1 and 2 respectively.
*/
static int getAutoVacuum(const char *z){
  int i;
  if CaseInsensitiveMatch(z, "none") {
	  return BTREE_AUTOVACUUM_NONE
	}
  if CaseInsensitiveMatch(z, "full") {
	  return BTREE_AUTOVACUUM_FULL
	}
  if CaseInsensitiveMatch(z, "incremental") {
	  return BTREE_AUTOVACUUM_INCR
	}
  i = sqlite3Atoi(z);
  return (byte)((i>=0&&i<=2)?i:0);
}

#ifndef SQLITE_OMIT_PAGER_PRAGMAS
/*
** Interpret the given string as a temp db location. Return 1 for file
** backed temporary databases, 2 for the Red-Black tree in memory database
** and 0 to use the compile-time default.
*/
static int getTempStore(const char *z){
  if( z[0]>='0' && z[0]<='2' ){
    return z[0] - '0';
  }else if CaseInsensitiveMatch(z, "file") {
    return 1;
  }else if CaseInsensitiveMatch(z, "memory") {
    return 2;
  }else{
    return 0;
  }
}
#endif /* SQLITE_PAGER_PRAGMAS */

#ifndef SQLITE_OMIT_PAGER_PRAGMAS
/*
** Invalidate temp storage, either when the temp storage is changed
** from default, or when 'file' and the temp_store_directory has changed
*/
static int invalidateTempStorage(Parse *pParse){
  sqlite3 *db = pParse.db;
  if( db.Databases[1].pBt!=0 ){
    if( !db.autoCommit || sqlite3BtreeIsInReadTrans(db.Databases[1].pBt) ){
      pParse.SetErrorMsg("temporary storage cannot be changed from within a transaction");
      return SQLITE_ERROR;
    }
    sqlite3BtreeClose(db.Databases[1].pBt);
    db.Databases[1].pBt = nil
    db.ResetInternalSchema(-1)
  }
  return SQLITE_OK;
}
#endif /* SQLITE_PAGER_PRAGMAS */

#ifndef SQLITE_OMIT_PAGER_PRAGMAS
/*
** If the TEMP database is open, close it and mark the database schema
** as needing reloading.  This must be done when using the SQLITE_TEMP_STORE
** or DEFAULT_TEMP_STORE pragmas.
*/
static int changeTempStorage(Parse *pParse, const char *zStorageType){
  int ts = getTempStore(zStorageType);
  sqlite3 *db = pParse.db;
  if( db.temp_store==ts ) return SQLITE_OK;
  if( invalidateTempStorage( pParse ) != SQLITE_OK ){
    return SQLITE_ERROR;
  }
  db.temp_store = (byte)ts;
  return SQLITE_OK;
}
#endif /* SQLITE_PAGER_PRAGMAS */

/*
** Generate code to return a single integer value.
*/
static void returnSingleInt(Parse *pParse, const char *zLabel, int64 value){
  Vdbe *v = pParse.GetVdbe()
  int mem = ++pParse.nMem;
  int64 *pI64 = sqlite3DbMallocRaw(pParse.db, sizeof(value));
  if( pI64 ){
    memcpy(pI64, &value, sizeof(value));
  }
  sqlite3VdbeAddOp4(v, OP_Int64, 0, mem, 0, (char*)pI64, P4_INT64);
  sqlite3VdbeSetNumCols(v, 1);
  sqlite3VdbeSetColName(v, 0, COLNAME_NAME, zLabel, SQLITE_STATIC);
  v.AddOp2(OP_ResultRow, mem, 1);
}

#ifndef SQLITE_OMIT_FLAG_PRAGMAS
/*
** Check to see if zRight and zLeft refer to a pragma that queries
** or changes one of the flags in db.flags.  Return 1 if so and 0 if not.
** Also, implement the pragma.
*/
static int flagPragma(Parse *pParse, const char *zLeft, const char *zRight){
  static const struct sPragmaType {
    const char *Name;  /* Name of the pragma */
    int mask;           /* Mask for the db.flags value */
  } aPragma[] = {
    { "full_column_names",        SQLITE_FullColNames  },
    { "short_column_names",       SQLITE_ShortColNames },
    { "count_changes",            SQLITE_CountRows     },
    { "empty_result_callbacks",   SQLITE_NullCallback  },
    { "legacy_file_format",       SQLITE_LegacyFileFmt },
    { "fullfsync",                SQLITE_FullFSync     },
    { "checkpoint_fullfsync",     SQLITE_CkptFullFSync },
    { "reverse_unordered_selects", SQLITE_ReverseOrder  },
    { "automatic_index",          SQLITE_AutoIndex     },
    { "ignore_check_constraints", SQLITE_IgnoreChecks  },
    /* The following is VERY experimental */
    { "writable_schema",          SQLITE_WriteSchema|SQLITE_RecoveryMode },

    /* TODO: Maybe it shouldn't be possible to change the ReadUncommitted
    ** flag if there are any active statements. */
    { "read_uncommitted",         SQLITE_ReadUncommitted },
    { "recursive_triggers",       SQLITE_RecTriggers },

    /* This flag may only be set if both foreign-key and trigger support
    ** are present in the build.  */
    { "foreign_keys",             SQLITE_ForeignKeys },
  };
  int i;
  const struct sPragmaType *p;
  for(i=0, p=aPragma; i<ArraySize(aPragma); i++, p++){
    if CaseInsensitiveMatch(zLeft, p.Name) {
      sqlite3 *db = pParse.db;
      Vdbe *v;
      v = pParse.GetVdbe()
      assert( v!=0 );  /* Already allocated by sqlite3Pragma() */
      if( v ){
        if( zRight==0 ){
          returnSingleInt(pParse, p.Name, (db.flags & p.mask)!=0 );
        }else{
          int mask = p.mask;          /* Mask of bits to set or clear. */
          if( db.autoCommit==0 ){
            /* Foreign key support may not be enabled or disabled while not
            ** in auto-commit mode.  */
            mask &= ~(SQLITE_ForeignKeys);
          }

          if( sqlite3GetBoolean(zRight, 0) ){
            db.flags |= mask;
          }else{
            db.flags &= ~mask;
          }

          //	Many of the flag-pragmas modify the code generated by the SQL compiler (eg. count_changes). So add an opcode to expire all compiled SQL statements after modifying a pragma value.
          v.AddOp2(OP_Expire, 0, 0);
        }
      }

      return 1;
    }
  }
  return 0;
}
#endif /* SQLITE_OMIT_FLAG_PRAGMAS */

/*
** Return a human-readable name for a constraint resolution action.
*/
static const char *actionName(byte action){
  const char *Name;
  switch( action ){
    case OE_SetNull:  Name = "SET NULL";        break;
    case OE_SetDflt:  Name = "SET DEFAULT";     break;
    case OE_Cascade:  Name = "CASCADE";         break;
    case OE_Restrict: Name = "RESTRICT";        break;
    default:          Name = "NO ACTION";  
                      assert( action==OE_None ); break;
  }
  return Name;
}


/*
** Parameter eMode must be one of the PAGER_JOURNALMODE_XXX constants
** defined in pager.h. This function returns the associated lowercase
** journal-mode name.
*/
const JOURNAL_MODES = []string{ "delete", "persist", "off", "truncate", "memory", "wal" }
//	assert( PAGER_JOURNALMODE_DELETE == 0 )
//	assert( PAGER_JOURNALMODE_PERSIST == 1 )
//	assert( PAGER_JOURNALMODE_OFF == 2 )
//	assert( PAGER_JOURNALMODE_TRUNCATE == 3 )
//	assert( PAGER_JOURNALMODE_MEMORY == 4 )
//	assert( PAGER_JOURNALMODE_WAL== 5 )

/*
** Process a pragma statement.  
**
** Pragmas are of this form:
**
**      PRAGMA [database.]id [= value]
**
** The identifier might also be a string.  The value is a string, and
** identifier, or a number.  If minusFlag is true, then the value is
** a number that was preceded by a minus sign.
**
** If the left side is "database.id" then pId1 is the database name
** and pId2 is the id.  If the left side is just "id" then pId1 is the
** id and pId2 is any empty string.
*/
 void sqlite3Pragma(
  Parse *pParse, 
  Token *pId1,        /* First part of [database.]id field */
  Token *pId2,        /* Second part of [database.]id field, or NULL */
  Token *pValue,      /* Token for <value>, or NULL */
  int minusFlag       /* True if a '-' sign preceded <value> */
){
  char *zLeft = 0;       /* Nul-terminated UTF-8 string <id> */
  char *zRight = 0;      /* Nul-terminated UTF-8 string <value>, or NULL */
  const char *zDb = 0;   /* The database name */
  Token *pId;            /* Pointer to <id> token */
  int iDb;               /* Database index for <database> */
  char *aFcntl[4];       /* Argument to SQLITE_FCNTL_PRAGMA */
  int rc;                      /* return value form SQLITE_FCNTL_PRAGMA */
  sqlite3 *db = pParse.db;    /* The database connection */
  Db *pDb;                     /* The specific database being pragmaed */
  Vdbe *v = pParse.pVdbe = sqlite3VdbeCreate(db);  /* Prepared statement */

  if( v==0 ) return;
  v.RunOnlyOnce()
  pParse.nMem = 2;

  /* Interpret the [database.] part of the pragma statement. iDb is the
  ** index of the database this pragma is being applied to in db.Databases[]. */
  pId, iDb = pParse.TwoPartName(pId1, pId2)
  if( iDb<0 ) return;
  pDb = &db.Databases[iDb];

  /* If the temp database has been explicitly named as part of the 
  ** pragma, make sure it is open. 
  */
  if( iDb==1 && sqlite3OpenTempDatabase(pParse) ){
    return;
  }

  zLeft = Dequote(pId)
  if( !zLeft ) return;
  if( minusFlag ){
    zRight = fmt.Sprintf("-%v", pValue);
  }else{
    zRight = Dequote(pValue)
  }

  assert( pId2 );
  zDb = pId2.n>0 ? pDb.Name : 0;
  if pParse.AuthCheck(SQLITE_PRAGMA, zLeft, zRight, zDb) {
    goto pragma_out;
  }

  /* Send an SQLITE_FCNTL_PRAGMA file-control to the underlying VFS
  ** connection.  If it returns SQLITE_OK, then assume that the VFS
  ** handled the pragma and generate a no-op prepared statement.
  */
  aFcntl[0] = 0;
  aFcntl[1] = zLeft;
  aFcntl[2] = zRight;
  aFcntl[3] = 0;
  rc = sqlite3_file_control(db, zDb, SQLITE_FCNTL_PRAGMA, (void*)aFcntl);
  if( rc==SQLITE_OK ){
    if( aFcntl[0] ){
      int mem = ++pParse.nMem;
      sqlite3VdbeAddOp4(v, OP_String8, 0, mem, 0, aFcntl[0], 0);
      sqlite3VdbeSetNumCols(v, 1);
      sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "result", SQLITE_STATIC);
      v.AddOp2(OP_ResultRow, mem, 1);
      aFcntl[0] = nil
    }
  }else if( rc!=SQLITE_NOTFOUND ){
    if( aFcntl[0] ){
      pParse.SetErrorMsg("%v", aFcntl[0]);
      aFcntl[0] = nil
    }
    pParse.nErr++;
    pParse.rc = rc;
  }else

#if !defined(SQLITE_OMIT_PAGER_PRAGMAS)
  /*
  **  PRAGMA [database.]page_size
  **  PRAGMA [database.]page_size=N
  **
  ** The first form reports the current setting for the
  ** database page size in bytes.  The second form sets the
  ** database page size value.  The value can only be set if
  ** the database has not yet been created.
  */
  if CaseInsensitiveMatch(zLeft,"page_size") {
    Btree *pBt = pDb.pBt;
    assert( pBt!=0 );
    if( !zRight ){
      int size = pBt ? sqlite3BtreeGetPageSize(pBt) : 0;
      returnSingleInt(pParse, "page_size", size);
    }else{
      /* Malloc may fail when setting the page-size, as there is an internal
      ** buffer that the pager module resizes using sqlite3_realloc().
      */
      db.nextPagesize = sqlite3Atoi(zRight);
      if( SQLITE_NOMEM==sqlite3BtreeSetPageSize(pBt, db.nextPagesize,-1,0) ){
        db.mallocFailed = true
      }
    }
  }else

  /*
  **  PRAGMA [database.]secure_delete
  **  PRAGMA [database.]secure_delete=ON/OFF
  **
  ** The first form reports the current setting for the
  ** secure_delete flag.  The second form changes the secure_delete
  ** flag setting and reports thenew value.
  */
  if CaseInsensitiveMatch(zLeft,"secure_delete") {
    Btree *pBt = pDb.pBt;
    int b = -1;
    assert( pBt!=0 );
    if( zRight ){
      b = sqlite3GetBoolean(zRight, 0);
    }
    if( pId2.n==0 && b>=0 ){
      int ii;
      for(i =, database :0 range db.Databases {
        sqlite3BtreeSecureDelete(db.Databases[ii].pBt, b);
      }
    }
    b = sqlite3BtreeSecureDelete(pBt, b);
    returnSingleInt(pParse, "secure_delete", b);
  }else

  /*
  **  PRAGMA [database.]max_page_count
  **  PRAGMA [database.]max_page_count=N
  **
  ** The first form reports the current setting for the
  ** maximum number of pages in the database file.  The 
  ** second form attempts to change this setting.  Both
  ** forms return the current setting.
  **
  ** The absolute value of N is used.  This is undocumented and might
  ** change.  The only purpose is to provide an easy way to test
  ** the sqlite3AbsInt32() function.
  **
  **  PRAGMA [database.]page_count
  **
  ** Return the number of pages in the specified database.
  */
  if CaseInsensitiveMatch(zLeft,"page_count") || CaseInsensitiveMatch(zLeft,"max_page_count") {
    int iReg;
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    pParse.CodeVerifySchema(iDb)
    iReg = ++pParse.nMem;
    if( sqlite3Tolower(zLeft[0])=='p' ){
      v.AddOp2(OP_Pagecount, iDb, iReg);
    }else{
      v.AddOp3(OP_MaxPgcnt, iDb, iReg, sqlite3AbsInt32(sqlite3Atoi(zRight)));
    }
    v.AddOp2(OP_ResultRow, iReg, 1);
    sqlite3VdbeSetNumCols(v, 1);
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, zLeft, SQLITE_TRANSIENT);
  }else

  /*
  **  PRAGMA [database.]locking_mode
  **  PRAGMA [database.]locking_mode = (normal|exclusive)
  */
  if CaseInsensitiveMatch(zLeft,"locking_mode") {
    const char *zRet = "normal";
    int eMode = getLockingMode(zRight);

    if( pId2.n==0 && eMode==PAGER_LOCKINGMODE_QUERY ){
      /* Simple "PRAGMA locking_mode;" statement. This is a query for
      ** the current default locking mode (which may be different to
      ** the locking-mode of the main database).
      */
      eMode = db.dfltLockMode;
    }else{
      Pager *pPager;
      if( pId2.n==0 ){
        /* This indicates that no database name was specified as part
        ** of the PRAGMA command. In this case the locking-mode must be
        ** set on all attached databases, as well as the main db file.
        **
        ** Also, the sqlite3.dfltLockMode variable is set so that
        ** any subsequently attached databases also use the specified
        ** locking mode.
        */
        int ii;
        assert(pDb==&db.Databases[0]);
        for(i =, database :2 range db.Databases {
          pPager = db.Databases[ii].pBt.Pager()
          sqlite3PagerLockingMode(pPager, eMode);
        }
        db.dfltLockMode = (byte)eMode;
      }
      pPager = pDb.pBt.Pager()
      eMode = sqlite3PagerLockingMode(pPager, eMode);
    }

    assert(eMode==PAGER_LOCKINGMODE_NORMAL||eMode==PAGER_LOCKINGMODE_EXCLUSIVE);
    if( eMode==PAGER_LOCKINGMODE_EXCLUSIVE ){
      zRet = "exclusive";
    }
    sqlite3VdbeSetNumCols(v, 1);
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "locking_mode", SQLITE_STATIC);
    sqlite3VdbeAddOp4(v, OP_String8, 0, 1, 0, zRet, 0);
    v.AddOp2(OP_ResultRow, 1, 1);
  }else

  /*
  **  PRAGMA [database.]journal_mode
  **  PRAGMA [database.]journal_mode =
  **                      (delete|persist|off|truncate|memory|wal|off)
  */
  if CaseInsensitiveMatch(zLeft,"journal_mode") {
    int eMode;        /* One of the PAGER_JOURNALMODE_XXX symbols */
    int ii;           /* Loop counter */

	//	Force the schema to be loaded on all databases. This causes all database files to be opened and the journal_modes set. This is necessary because subsequent processing must know if the databases are in WAL mode.
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}

    sqlite3VdbeSetNumCols(v, 1);
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "journal_mode", SQLITE_STATIC);

    if( zRight==0 ){
      /* If there is no "=MODE" part of the pragma, do a query for the
      ** current mode */
      eMode = PAGER_JOURNALMODE_QUERY;
    }else{
      const char *zMode;
      int n = sqlite3Strlen30(zRight);
      for(eMode=0; (zMode = JOURNAL_MODES[eMode]) != 0; eMode++){
        if CaseInsensitiveMatchN(zRight, zMode, n) {
			break
		}
      }
      if( !zMode ){
        /* If the "=MODE" part does not match any known journal mode,
        ** then do a query */
        eMode = PAGER_JOURNALMODE_QUERY;
      }
    }
    if( eMode==PAGER_JOURNALMODE_QUERY && pId2.n==0 ){
      /* Convert "PRAGMA journal_mode" into "PRAGMA main.journal_mode" */
      iDb = 0;
      pId2.n = 1;
    }
    f, database :o range db.Databases {
      if( db.Databases[ii].pBt && (ii==iDb || pId2.n==0) ){
        sqlite3VdbeUsesBtree(v, ii);
        v.AddOp3(OP_JournalMode, ii, 1, eMode);
      }
    }
    v.AddOp2(OP_ResultRow, 1, 1);
  }else

  /*
  **  PRAGMA [database.]journal_size_limit
  **  PRAGMA [database.]journal_size_limit=N
  **
  ** Get or set the size limit on rollback journal files.
  */
  if CaseInsensitiveMatch(zLeft,"journal_size_limit") {
    Pager *pPager = pDb.pBt.Pager()
    int64 iLimit = -2;
    if( zRight ){
      Atoint64(zRight, &iLimit, 1000000, SQLITE_UTF8);
      if( iLimit<-1 ) iLimit = -1;
    }
    iLimit = sqlite3PagerJournalSizeLimit(pPager, iLimit);
    returnSingleInt(pParse, "journal_size_limit", iLimit);
  }else

#endif /* SQLITE_OMIT_PAGER_PRAGMAS */

  /*
  **  PRAGMA [database.]auto_vacuum
  **  PRAGMA [database.]auto_vacuum=N
  **
  ** Get or set the value of the database 'auto-vacuum' parameter.
  ** The value is one of:  0 NONE 1 FULL 2 INCREMENTAL
  */
  if CaseInsensitiveMatch(zLeft,"auto_vacuum") {
    Btree *pBt = pDb.pBt;
    assert( pBt!=0 );
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    if( !zRight ){
      int auto_vacuum;
      if( pBt ){
         auto_vacuum = sqlite3BtreeGetAutoVacuum(pBt);
      }else{
         auto_vacuum = SQLITE_DEFAULT_AUTOVACUUM;
      }
      returnSingleInt(pParse, "auto_vacuum", auto_vacuum);
    }else{
		eAuto := getAutoVacuum(zRight)
		assert( eAuto >= 0 && eAuto <= 2 )
		db.nextAutovac = byte(eAuto)
		if eAuto >= 0 {
			//	Call SetAutoVacuum() to set initialize the internal auto and incr-vacuum flags. This is required in case this connection creates the database file. It is important that it is created as an auto-vacuum capable db.
        	rc = sqlite3BtreeSetAutoVacuum(pBt, eAuto);
			if rc == SQLITE_OK && eAuto == 1 || eAuto == 2 {
				iAddr := v.AddOpList(AUTO_VACUUM_SET_META_6...)
				v.ChangeP1(iAddr, iDb)
				v.ChangeP1(iAddr + 1, iDb)
				v.ChangeP2(iAddr + 2, iAddr + 4)
				v.ChangeP1(iAddr + 4, eAuto - 1)
				v.ChangeP1(iAddr + 5, iDb)
				sqlite3VdbeUsesBtree(v, iDb)
			}
      	}
    }
  }else

  /*
  **  PRAGMA [database.]incremental_vacuum(N)
  **
  ** Do N steps of incremental vacuuming on a database.
  */
  if CaseInsensitiveMatch(zLeft,"incremental_vacuum") {
    int iLimit, addr;
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    if( zRight==0 || !sqlite3GetInt32(zRight, &iLimit) || iLimit<=0 ){
      iLimit = 0x7fffffff;
    }
    pParse.BeginWriteOperation(0, iDb)
    v.AddOp2(OP_Integer, iLimit, 1);
    addr = v.AddOp1(OP_IncrVacuum, iDb);
    v.AddOp1(OP_ResultRow, 1);
    v.AddOp2(OP_AddImm, 1, -1);
    v.AddOp2(OP_IfPos, 1, addr);
    v.JumpHere(addr)
  }else

#ifndef SQLITE_OMIT_PAGER_PRAGMAS
  /*
  **  PRAGMA [database.]cache_size
  **  PRAGMA [database.]cache_size=N
  **
  ** The first form reports the current local setting for the
  ** page cache size. The second form sets the local
  ** page cache size value.  If N is positive then that is the
  ** number of pages in the cache.  If N is negative, then the
  ** number of pages is adjusted so that the cache uses -N kibibytes
  ** of memory.
  */
  if CaseInsensitiveMatch(zLeft,"cache_size") {
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    if( !zRight ){
      returnSingleInt(pParse, "cache_size", pDb.Schema.cache_size);
    }else{
      int size = sqlite3Atoi(zRight);
      pDb.Schema.cache_size = size;
      sqlite3BtreeSetCacheSize(pDb.pBt, pDb.Schema.cache_size);
    }
  }else

  /*
  **   PRAGMA temp_store
  **   PRAGMA temp_store = "default"|"memory"|"file"
  **
  ** Return or set the local value of the temp_store flag.  Changing
  ** the local value does not make changes to the disk file and the default
  ** value will be restored the next time the database is opened.
  **
  ** Note that it is possible for the library compile-time options to
  ** override this setting
  */
  if CaseInsensitiveMatch(zLeft, "temp_store") {
    if( !zRight ){
      returnSingleInt(pParse, "temp_store", db.temp_store);
    }else{
      changeTempStorage(pParse, zRight);
    }
  }else

  /*
  **   PRAGMA temp_store_directory
  **   PRAGMA temp_store_directory = ""|"directory_name"
  **
  ** Return or set the local value of the temp_store_directory flag.  Changing
  ** the value sets a specific directory to be used for temporary files.
  ** Setting to a null string reverts to the default temporary directory search.
  ** If temporary directory is changed, then invalidateTempStorage.
  **
  */
  if CaseInsensitiveMatch(zLeft, "temp_store_directory") {
    if( !zRight ){
      if( sqlite3_temp_directory ){
        sqlite3VdbeSetNumCols(v, 1);
        sqlite3VdbeSetColName(v, 0, COLNAME_NAME, 
            "temp_store_directory", SQLITE_STATIC);
        sqlite3VdbeAddOp4(v, OP_String8, 0, 1, 0, sqlite3_temp_directory, 0);
        v.AddOp2(OP_ResultRow, 1, 1);
      }
    }else{
      if( zRight[0] ){
        int res;
        rc = sqlite3OsAccess(db.pVfs, zRight, SQLITE_ACCESS_READWRITE, &res);
        if( rc!=SQLITE_OK || res==0 ){
          pParse.SetErrorMsg("not a writable directory");
          goto pragma_out;
        }
      }
      if( SQLITE_TEMP_STORE==0
       || (SQLITE_TEMP_STORE==1 && db.temp_store<=1)
       || (SQLITE_TEMP_STORE==2 && db.temp_store==1)
      ){
        invalidateTempStorage(pParse);
      }
      sqlite3_temp_directory = nil
      if( zRight[0] ){
        sqlite3_temp_directory = fmt.Sprintf("%v", zRight);
      }else{
        sqlite3_temp_directory = 0;
      }
    }
  }else

  /*
  **   PRAGMA [database.]synchronous
  **   PRAGMA [database.]synchronous=OFF|ON|NORMAL|FULL
  **
  ** Return or set the local value of the synchronous flag.  Changing
  ** the local value does not make changes to the disk file and the
  ** default value will be restored the next time the database is
  ** opened.
  */
  if CaseInsensitiveMatch(zLeft,"synchronous") {
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    if( !zRight ){
      returnSingleInt(pParse, "synchronous", pDb.safety_level-1);
    }else{
      if( !db.autoCommit ){
        pParse, "Safety level may not be changed inside a transaction");
      }else{
        pDb.safety_level = getSafetyLevel(zRight,0,1)+1;
      }
    }
  }else
#endif /* SQLITE_OMIT_PAGER_PRAGMAS */

#ifndef SQLITE_OMIT_FLAG_PRAGMAS
  if( flagPragma(pParse, zLeft, zRight) ){
    /* The flagPragma() subroutine also generates any necessary code
    ** there is nothing more to do here */
  }else
#endif /* SQLITE_OMIT_FLAG_PRAGMAS */

#ifndef SQLITE_OMIT_SCHEMA_PRAGMAS
  /*
  **   PRAGMA table_info(<table>)
  **
  ** Return a single row for each column of the named table. The columns of
  ** the returned data set are:
  **
  ** cid:        Column id (numbered from left to right, starting at 0)
  ** name:       Column name
  ** type:       Column declaration type.
  ** notnull:    True if 'NOT NULL' is part of column declaration
  ** dflt_value: The default value for the column, if any.
  */
  if CaseInsensitiveMatch(zLeft, "table_info") && zRight != "" {
    Table *pTab;
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    pTab = db.FindTable(zRight, zDb)
    if( pTab ){
      int i;
      int nHidden = 0;
      Column *pCol;
      sqlite3VdbeSetNumCols(v, 6);
      pParse.nMem = 6;
      sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "cid", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "name", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "type", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 3, COLNAME_NAME, "notnull", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 4, COLNAME_NAME, "dflt_value", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 5, COLNAME_NAME, "pk", SQLITE_STATIC);
      sqlite3ViewGetColumnNames(pParse, pTab);
      for(i=0, pCol=pTab.Columns; i<pTab.nCol; i++, pCol++){
        if pCol.IsHidden {
          nHidden++;
          continue;
        }
        v.AddOp2(OP_Integer, i-nHidden, 1);
        sqlite3VdbeAddOp4(v, OP_String8, 0, 2, 0, pCol.Name, 0);
        sqlite3VdbeAddOp4(v, OP_String8, 0, 3, 0,
           pCol.zType ? pCol.zType : "", 0);
        v.AddOp2(OP_Integer, (pCol.notNull ? 1 : 0), 4);
        if( pCol.zDflt ){
          sqlite3VdbeAddOp4(v, OP_String8, 0, 5, 0, (char*)pCol.zDflt, 0);
        }else{
          v.AddOp2(OP_Null, 0, 5);
        }
        v.AddOp2(OP_Integer, pCol.isPrimKey, 6);
        v.AddOp2(OP_ResultRow, 1, 6);
      }
    }
  }else

  if CaseInsensitiveMatch(zLeft, "index_info") && zRight != "" {
    Index *pIdx;
    Table *pTab;
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    pIdx = db.FindIndex(zRight, zDb)
    if( pIdx ){
      int i;
      pTab = pIdx.pTable;
      sqlite3VdbeSetNumCols(v, 3);
      pParse.nMem = 3;
      sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "seqno", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "cid", SQLITE_STATIC);
      sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "name", SQLITE_STATIC);
	  for i, column := range pIdx.Columns {
        v.AddOp2(OP_Integer, i, 1);
        v.AddOp2(OP_Integer, column, 2);
        assert( pTab.nCol > column );
        sqlite3VdbeAddOp4(v, OP_String8, 0, 3, 0, pTab.Columns[column].Name, 0);
        v.AddOp2(OP_ResultRow, 1, 3);
      }
    }
  }else

	if CaseInsensitiveMatch(zLeft, "index_list") && zRight != "" {
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
		if table := db.FindTable(zRight, zDb); pTab != nil {
			v = pParse.GetVdbe()
			if len(table.Indices) > 0 {
				sqlite3VdbeSetNumCols(v, 3)
				pParse.nMem = 3
				sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "seq", SQLITE_STATIC)
				sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "name", SQLITE_STATIC)
				sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "unique", SQLITE_STATIC)
				for i, index := range table.Indices {
					v.AddOp2(OP_Integer, i, 1)
					sqlite3VdbeAddOp4(v, OP_String8, 0, 2, 0, index.Name, 0)
					v.AddOp2(OP_Integer, index.onError != OE_None, 3)
					v.AddOp2(OP_ResultRow, 1, 3)
				}
			}
		}
  }else

  if CaseInsensitiveMatch(zLeft, "database_list") {
    int i;
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    sqlite3VdbeSetNumCols(v, 3);
    pParse.nMem = 3;
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "seq", SQLITE_STATIC);
    sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "name", SQLITE_STATIC);
    sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "file", SQLITE_STATIC);
	for i, database := range db.Databases {
		if database.pBt != nil {
			assert( database.Name != "" )
			v.AddOp2(OP_Integer, i, 1)
			sqlite3VdbeAddOp4(v, OP_String8, 0, 2, 0, database.Name, 0)
			sqlite3VdbeAddOp4(v, OP_String8, 0, 3, 0, sqlite3BtreeGetFilename(database.pBt), 0)
			v.AddOp2(OP_ResultRow, 1, 3)
		}
	}
  }else

	if CaseInsensitiveMatch(zLeft, "collation_list") {
		sqlite3VdbeSetNumCols(v, 2)
		pParse.nMem = 2
		sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "seq", SQLITE_STATIC)
		sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "name", SQLITE_STATIC)
		i := 0
		for _, pColl := range db.Collations) {
			v.AddOp2(OP_Integer, i, 1)
			i++
			sqlite3VdbeAddOp4(v, OP_String8, 0, 2, 0, pColl.Name, 0)
			v.AddOp2(OP_ResultRow, 1, 2)
		}
  }else
#endif /* SQLITE_OMIT_SCHEMA_PRAGMAS */

  if CaseInsensitiveMatch(zLeft, "foreign_key_list") && zRight != "" {
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
    if pTab := db.FindTable(zRight, zDb); pTab != nil {
      v = pParse.GetVdbe()
      if pFK := pTab.ForeignKey; pFK != nil {
        int i = 0; 
        sqlite3VdbeSetNumCols(v, 8);
        pParse.nMem = 8;
        sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "id", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "seq", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "table", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 3, COLNAME_NAME, "from", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 4, COLNAME_NAME, "to", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 5, COLNAME_NAME, "on_update", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 6, COLNAME_NAME, "on_delete", SQLITE_STATIC);
        sqlite3VdbeSetColName(v, 7, COLNAME_NAME, "match", SQLITE_STATIC);
        while(pFK){
          int j;
          for(j=0; j<pFK.nCol; j++){
            char *zCol = pFK.Columns[j].zCol;
            char *zOnDelete = (char *)actionName(pFK.aAction[0]);
            char *zOnUpdate = (char *)actionName(pFK.aAction[1]);
            v.AddOp2(OP_Integer, i, 1);
            v.AddOp2(OP_Integer, j, 2);
            sqlite3VdbeAddOp4(v, OP_String8, 0, 3, 0, pFK.zTo, 0);
            sqlite3VdbeAddOp4(v, OP_String8, 0, 4, 0,
                              pTab.Columns[pFK.Columns[j].iFrom].Name, 0);
            sqlite3VdbeAddOp4(v, zCol ? OP_String8 : OP_Null, 0, 5, 0, zCol, 0);
            sqlite3VdbeAddOp4(v, OP_String8, 0, 6, 0, zOnUpdate, 0);
            sqlite3VdbeAddOp4(v, OP_String8, 0, 7, 0, zOnDelete, 0);
            sqlite3VdbeAddOp4(v, OP_String8, 0, 8, 0, "NONE", 0);
            v.AddOp2(OP_ResultRow, 1, 8);
          }
          ++i;
          pFK = pFK.NextFrom;
        }
      }
    }
  }else

  /* Reinstall the LIKE and GLOB functions.  The variant of LIKE
  ** used will be case sensitive or not depending on the RHS.
  */
  if CaseInsensitiveMatch(zLeft, "case_sensitive_like") {
    if( zRight ){
      sqlite3RegisterLikeFunctions(db, sqlite3GetBoolean(zRight, 0));
    }
  }else

#ifndef SQLITE_INTEGRITY_CHECK_ERROR_MAX
# define SQLITE_INTEGRITY_CHECK_ERROR_MAX 100
#endif

	//	Pragma "quick_check" is an experimental reduced version of integrity_check designed to detect most database corruption without most of the overhead of a full integrity-check.
	if CaseInsensitiveMatch(zLeft, "integrity_check") || CaseInsensitiveMatch(zLeft, "quick_check") {
		int i, j, addr, mxErr

		isQuick := sqlite3Tolower(zLeft[0]) == 'q'
		//	Initialize the VDBE program
		if pParse.ReadSchema() != SQLITE_OK {
			goto pragma_out
		}
		pParse.nMem = 6
		sqlite3VdbeSetNumCols(v, 1)
		sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "integrity_check", SQLITE_STATIC)

		//	Set the maximum error count
		mxErr = SQLITE_INTEGRITY_CHECK_ERROR_MAX
		if zRight != nil {
			sqlite3GetInt32(zRight, &mxErr)
			if mxErr <= 0 {
				mxErr = SQLITE_INTEGRITY_CHECK_ERROR_MAX
			}
		}
		v.AddOp2(OP_Integer, mxErr, 1)					//	reg[1] holds errors left

		//	Do an integrity check on each database file
		for _, database := range db.Databases {
			int cnt = 0
			pParse.CodeVerifySchema(i)
			addr = v.AddOp1(OP_IfPos, 1)				//	Halt if out of errors
			v.AddOp2(OP_Halt, 0, 0)
			v.JumpHere(addr)

			//	Do an integrity check of the B-Tree
			//	Begin by filling registers 2, 3, ... with the root pages numbers for all tables and indices in the database.
			pTbls := &database.Schema.Tables
			for _, pTab := range pTbls {
				v.AddOp2(OP_Integer, pTab.tnum, 2 + cnt)
				cnt++
				for _, index := range pTab.Indices {
					v.AddOp2(OP_Integer, index.tnum, 2 + cnt)
					cnt++
				}
			}

			//	Make sure sufficient number of registers have been allocated
			if pParse.nMem < cnt + 4 {
				pParse.nMem = cnt + 4
			}

		     //	Do the b-tree integrity checks
			v.AddOp3(OP_IntegrityCheck, 2, cnt, 1)
			v.ChangeP5(byte(i))
			addr = v.AddOp1(OP_IsNull, 2)
			sqlite3VdbeAddOp4(v, OP_String8, 0, 3, 0, fmt.Sprintf("*** in database %v ***\n", database.Name), P4_DYNAMIC)
			v.AddOp3(OP_Move, 2, 4, 1)
			v.AddOp3(OP_Concat, 4, 3, 2)
			v.AddOp2(OP_ResultRow, 2, 1)
			v.JumpHere(addr)

			//	Make sure all the indices are constructed correctly.
			if !isQuick {
				for _, table := range pTbls {
					if len(table.Indices) > 0 {
						addr = v.AddOp1(OP_IfPos, 1)			//	Stop if out of errors
						v.AddOp2(OP_Halt, 0, 0)
						v.JumpHere(addr)
						pParse.OpenTableAndIndices(pTab, 1, OP_OpenRead)
						v.AddOp2(OP_Integer, 0, 2)				//	reg(2) will count entries
						loopTop := v.AddOp2(OP_Rewind, 1, 0)
						v.AddOp2(OP_AddImm, 2, 1)				//	increment entry count
						for j, index := range table.Indices {
							r1 := sqlite3GenerateIndexKey(pParse, index, 1, 3, 0)
							jmp2 := sqlite3VdbeAddOp4Int(v, OP_Found, j + 2, 0, r1, len(index.Columns) + 1)
							addr = v.AddOpList(INTEGRITY_CHECK_INDEX_ERROR...)
							sqlite3VdbeChangeP4(v, addr + 1, "rowid ", P4_STATIC)
							sqlite3VdbeChangeP4(v, addr + 3, " missing from index ", P4_STATIC)
							sqlite3VdbeChangeP4(v, addr + 4, pIdx.Name, P4_TRANSIENT)
							v.JumpHere(addr + 9)
							v.JumpHere(jmp2)
						}
						v.AddOp2(OP_Next, 1, loopTop+1);
						v.JumpHere(loopTop)
						for j, index := range table.Indices {
							addr = v.AddOp1(OP_IfPos, 1)
							v.AddOp2(OP_Halt, 0, 0)
							v.JumpHere(addr)
							addr = v.AddOpList(INTEGRITY_CHECK_COUNT_INDICES...)
							v.ChangeP1(addr + 1, j + 2)
							v.ChangeP2(addr + 1, addr + 4)
							v.ChangeP1(addr + 3, j + 2)
							v.ChangeP2(addr + 3, addr + 2)
							v.JumpHere(addr + 4)
							sqlite3VdbeChangeP4(v, addr + 6, "wrong # of entries in index ", P4_STATIC)
							sqlite3VdbeChangeP4(v, addr + 7, index.Name, P4_TRANSIENT)
						}
					}
				}
			}
		}
		addr = v.AddOpList(COMPLETED_INTEGRITY_CHECK...)
		v.ChangeP2(addr, -mxErr)
		v.JumpHere(addr + 1)
		sqlite3VdbeChangeP4(v, addr+2, "ok", P4_STATIC)
	}else
#ifndef SQLITE_OMIT_SCHEMA_VERSION_PRAGMAS
  /*
  **   PRAGMA [database.]schema_version
  **   PRAGMA [database.]schema_version = <integer>
  **
  **   PRAGMA [database.]user_version
  **   PRAGMA [database.]user_version = <integer>
  **
  ** The pragma's schema_version and user_version are used to set or get
  ** the value of the schema-version and user-version, respectively. Both
  ** the schema-version and the user-version are 32-bit signed integers
  ** stored in the database header.
  **
  ** The schema-cookie is usually only manipulated internally by SQLite. It
  ** is incremented by SQLite whenever the database schema is modified (by
  ** creating or dropping a table or index). The schema version is used by
  ** SQLite each time a query is executed to ensure that the internal cache
  ** of the schema used when compiling the SQL query matches the schema of
  ** the database against which the compiled query is actually executed.
  ** Subverting this mechanism by using "PRAGMA schema_version" to modify
  ** the schema-version is potentially dangerous and may lead to program
  ** crashes or database corruption. Use with caution!
  **
  ** The user-version is not used internally by SQLite. It may be used by
  ** applications for any purpose.
  */
  if CaseInsensitiveMatch(zLeft, "schema_version") || CaseInsensitiveMatch(zLeft, "user_version") || CaseInsensitiveMatch(zLeft, "freelist_count") {
    int iCookie;   /* Cookie index. 1 for schema-cookie, 6 for user-cookie. */
    sqlite3VdbeUsesBtree(v, iDb);
    switch( zLeft[0] ){
      case 'f': case 'F':
        iCookie = BTREE_FREE_PAGE_COUNT;
        break;
      case 's': case 'S':
        iCookie = BTREE_SCHEMA_VERSION;
        break;
      default:
        iCookie = BTREE_USER_VERSION;
        break;
    }

	if zRight && iCookie != BTREE_FREE_PAGE_COUNT {
		addr := v.AddOpList(SET_COOKIE...)
		v.ChangeP1(addr, iDb)
		v.ChangeP1(addr + 1, sqlite3Atoi(zRight))
		v.ChangeP1(addr + 2, iDb)
		v.ChangeP2(addr + 2, iCookie)
	} else {
		addr := v.AddOpList(READ_COOKIE...)
		v.ChangeP1(addr, iDb)
		v.ChangeP1(addr + 1, iDb)
		v.ChangeP3(addr + 1, iCookie)
		sqlite3VdbeSetNumCols(v, 1);
		sqlite3VdbeSetColName(v, 0, COLNAME_NAME, zLeft, SQLITE_TRANSIENT);
    }
  }else
#endif /* SQLITE_OMIT_SCHEMA_VERSION_PRAGMAS */

#ifndef SQLITE_OMIT_COMPILEOPTION_DIAGS
  /*
  **   PRAGMA compile_options
  **
  ** Return the names of all compile-time options used in this build,
  ** one option per row.
  */
  if CaseInsensitiveMatch(zLeft, "compile_options") {
    int i = 0;
    const char *zOpt;
    sqlite3VdbeSetNumCols(v, 1);
    pParse.nMem = 1;
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "compile_option", SQLITE_STATIC);
    while( (zOpt = sqlite3_compileoption_get(i++))!=0 ){
      sqlite3VdbeAddOp4(v, OP_String8, 0, 1, 0, zOpt, 0);
      v.AddOp2(OP_ResultRow, 1, 1);
    }
  }else
#endif /* SQLITE_OMIT_COMPILEOPTION_DIAGS */

  /*
  **   PRAGMA [database.]wal_checkpoint = passive|full|restart
  **
  ** Checkpoint the database.
  */
  if CaseInsensitiveMatch(zLeft, "wal_checkpoint") {
    int iBt = (pId2.z?iDb:SQLITE_MAX_ATTACHED);
    int eMode = SQLITE_CHECKPOINT_PASSIVE;
    if( zRight ){
      if CaseInsensitiveMatch(zRight, "full") {
        eMode = SQLITE_CHECKPOINT_FULL;
      }else if CaseInsensitiveMatch(zRight, "restart") {
        eMode = SQLITE_CHECKPOINT_RESTART;
      }
    }
    if pParse.ReadSchema() != SQLITE_OK {
		goto pragma_out
	}
    sqlite3VdbeSetNumCols(v, 3);
    pParse.nMem = 3;
    sqlite3VdbeSetColName(v, 0, COLNAME_NAME, "busy", SQLITE_STATIC);
    sqlite3VdbeSetColName(v, 1, COLNAME_NAME, "log", SQLITE_STATIC);
    sqlite3VdbeSetColName(v, 2, COLNAME_NAME, "checkpointed", SQLITE_STATIC);

    v.AddOp3(OP_Checkpoint, iBt, eMode, 1);
    v.AddOp2(OP_ResultRow, 1, 3);
  }else

  /*
  **   PRAGMA wal_autocheckpoint
  **   PRAGMA wal_autocheckpoint = N
  **
  ** Configure a database connection to automatically checkpoint a database
  ** after accumulating N frames in the log. Or query for the current value
  ** of N.
  */
  if CaseInsensitiveMatch(zLeft, "wal_autocheckpoint") {
    if( zRight ){
      sqlite3_wal_autocheckpoint(db, sqlite3Atoi(zRight));
    }
    returnSingleInt(pParse, "wal_autocheckpoint", 
       db.xWalCallback==sqlite3WalDefaultHook ? 
           SQLITE_PTR_TO_INT(db.pWalArg) : 0);
  }else

  /*
  **  PRAGMA shrink_memory
  **
  ** This pragma attempts to free as much memory as possible from the
  ** current database connection.
  */
  if CaseInsensitiveMatch(zLeft, "shrink_memory") {
    db.db_release_memory()
  } else if CaseInsensitiveMatch(zLeft, "key") && zRight != "" {
    sqlite3_key(db, zRight, sqlite3Strlen30(zRight));
  }else if CaseInsensitiveMatch(zLeft, "rekey") && zRight != "" {
    sqlite3_rekey(db, zRight, sqlite3Strlen30(zRight));
  }else if zRight != "" && (CaseInsensitiveMatch(zLeft, "hexkey") || CaseInsensitiveMatch(zLeft, "hexrekey")) {
    int i, h1, h2;
    char zKey[40];
    for(i=0; (h1 = zRight[i])!=0 && (h2 = zRight[i+1])!=0; i+=2){
      h1 += 9*(1&(h1>>6));
      h2 += 9*(1&(h2>>6));
      zKey[i/2] = (h2 & 0x0f) | ((h1 & 0xf)<<4);
    }
    if( (zLeft[3] & 0xf)==0xb ){
      sqlite3_key(db, zKey, i/2);
    }else{
      sqlite3_rekey(db, zKey, i/2);
    }
  }else
#if defined(SQLITE_ENABLE_CEROD)
  if CaseInsensitiveMatch(zLeft, "activate_extensions") {
    if CaseInsensitiveMatchN(zRight, "see-", 4) {
      sqlite3_activate_see(&zRight[4]);
    }
#ifdef SQLITE_ENABLE_CEROD
    if CaseInsensitiveMatchN(zRight, "cerod-", 6) {
      sqlite3_activate_cerod(&zRight[6]);
    }
#endif
  }else
#endif

 
  {/* Empty ELSE clause */}

  /*
  ** Reset the safety level, in case the fullfsync flag or synchronous
  ** setting changed.
  */
#ifndef SQLITE_OMIT_PAGER_PRAGMAS
  if( db.autoCommit ){
    sqlite3BtreeSetSafetyLevel(pDb.pBt, pDb.safety_level,
               (db.flags&SQLITE_FullFSync)!=0,
               (db.flags&SQLITE_CkptFullFSync)!=0);
  }
#endif
pragma_out:
  zLeft = nil
  zRight = nil
}

#endif /* SQLITE_OMIT_PRAGMA */
