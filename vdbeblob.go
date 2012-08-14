/* This file contains code used to implement incremental BLOB I/O.
*/


/*
** Valid sqlite3_blob* handles point to Incrblob structures.
*/
typedef struct Incrblob Incrblob;
struct Incrblob {
  int flags;              /* Copy of "flags" passed to sqlite3_blob_open() */
  int nByte;              /* Size of open blob, in bytes */
  int iOffset;            /* Byte offset of blob in cursor data */
  int iCol;               /* Table column this handle is open on */
  btree.Cursor *pCsr;         /* Cursor pointing at blob row */
  sqlite3_stmt *pStmt;    /* Statement holding cursor open */
  sqlite3 *db;            /* The associated database */
};


//	This function is used by both blob_open() and blob_reopen(). It seeks the b-tree cursor associated with blob handle p to point to row iRow. If successful, SQLITE_OK is returned and subsequent calls to sqlite3_blob_read() or sqlite3_blob_write() access the specified row.
//	If an error occurs, or if the specified row does not exist or does not contain a value of type TEXT or BLOB in the column nominated when the blob handle was opened, then an error code is returned and *pzErr may be set to point to a buffer containing an error message.
//	If an error does occur, then the b-tree cursor is closed. All subsequent calls to sqlite3_blob_read(), blob_write() or blob_reopen() will immediately return SQLITE_ABORT.
func (p *Incrblob) SeekToRow(iRow int64) (rc int, Err string) {
	v := (Vdbe *)(p.pStmt)
	//	Set the value of the SQL statements only variable to integer iRow. This is done directly instead of using sqlite3_bind_int64() to avoid triggering asserts related to mutexes.
	v.aVar[0].Store(iRow)
	if rc = sqlite3_step(p.pStmt); rc == SQLITE_ROW {
		if Type := v.apCsr[0].aType[p.iCol]; Type < 12 {
			switch Type {
			case 0:
				Err = fmt.Sprintf("cannot open value of type null")
			case 7:
				Err = fmt.Sprintf("cannot open value of type real")
			default:
				Err = fmt.Sprintf("cannot open value of type integer")
			}
			rc = SQLITE_ERROR
			sqlite3_finalize(p.pStmt)
			p.pStmt = nil
		} else {
			p.iOffset = v.apCsr[0].aOffset[p.iCol]
			p.nByte = VdbeSerialTypeLen(Type)
			p.pCsr =  v.apCsr[0].pCursor
			p.pCsr.CacheOverflow()
		}
	}

	switch {
	case rc == SQLITE_ROW:
		rc = SQLITE_OK
	case p.pStmt != nil:
		rc = sqlite3_finalize(p.pStmt)
		p.pStmt = nil
		if rc == SQLITE_OK {
			Err = fmt.Sprintf("no such rowid: %v", iRow)
			rc = SQLITE_ERROR
		} else {
			Err = fmt.Sprintf("%v", sqlite3_errmsg(p.db))
		}
	}

	assert( rc != SQLITE_OK || zErr == "" )
	assert( rc != SQLITE_ROW && rc != SQLITE_DONE )
	return
}

/*
** Open a blob handle.
*/
func int sqlite3_blob_open(
  sqlite3* db,            /* The database connection */
  const char *zDb,        /* The attached database containing the blob */
  const char *zTable,     /* The table containing the blob */
  const char *zColumn,    /* The column containing the blob */
  sqlite_int64 iRow,      /* The row containing the glob */
  int flags,              /* True . read/write access, false . read-only */
  sqlite3_blob **ppBlob   /* Handle for accessing the blob returned here */
){
  int nAttempt = 0;
  int iCol;               /* Index of zColumn in row-record */

  int rc = SQLITE_OK;
  char *zErr = 0;
  Table *pTab;
  Parse *pParse = 0;
  Incrblob *pBlob = 0;

  flags = !!flags;                /* flags = (flags ? 1 : 0); */
  *ppBlob = 0;

  db.mutex.Lock()

  pBlob = (Incrblob *)sqlite3DbMallocZero(db, sizeof(Incrblob));
  if( !pBlob ) goto blob_open_out;
  pParse = sqlite3StackAllocRaw(db, sizeof(*pParse));
  if( !pParse ) goto blob_open_out;

  do {
    memset(pParse, 0, sizeof(Parse));
    pParse.db = db;
    zErr = nil

    db.LockAll()
    pTab = pParse.LocateTable(zTable, zDb, false)
    if( pTab && pTab.IsVirtual() ){
      pTab = 0;
      pParse.SetErrorMsg("cannot open virtual table: %v", zTable);
    }
    if( pTab && pTab.Select ){
      pTab = 0;
      pParse.SetErrorMsg("cannot open view: %v", zTable);
    }
    if( !pTab ){
      if( pParse.zErrMsg ){
        zErr = pParse.zErrMsg;
        pParse.zErrMsg = 0;
      }
      rc = SQLITE_ERROR;
      db.UnlockAll()
      goto blob_open_out;
    }

    /* Now search pTab for the exact column. */
    for(iCol=0; iCol<pTab.nCol; iCol++) {
      if CaseInsensitiveMatch(pTab.Columns[iCol].Name, zColumn) {
        break;
      }
    }
    if( iCol==pTab.nCol ){
      zErr = fmt.Sprintf("no such column: \"%v\"", zColumn);
      rc = SQLITE_ERROR;
      db.UnlockAll()
      goto blob_open_out;
    }

    //	If the value is being opened for writing, check that the column is not indexed, and that it is not part of a foreign key. It is against the rules to open a column to which either of these descriptions applies for writing.
    if flags != 0 {
      zFault	string
      if( db.flags&SQLITE_ForeignKeys ){
        //	Check that the column is not part of an FK child key definition. It is not necessary to check if it is part of a parent key, as parent key columns must be indexed. The check below will pick up this case.
        for f := pTab.ForeignKey; f != nil; f = f.NextFrom {
          for j := 0; j < f.nCol; j++ {
            if f.Columns[j].iFrom == iCol {
              zFault = "foreign key"
            }
          }
        }
      }
	  for _, index := range pTab.Indices {
		  for _, column := range index.Columns {
			  if column == iCol {
				  zFault = "indexed"
			  }
		  }
	  }
      if zFault != "" {
        zErr = fmt.Sprintf("cannot open %v column for writing", zFault)
        rc = SQLITE_ERROR
        db.UnlockAll()
        goto blob_open_out
      }
    }

    pBlob.pStmt = (sqlite3_stmt *)sqlite3VdbeCreate(db);
    assert( pBlob.pStmt || db.mallocFailed );
    if( pBlob.pStmt ){
      Vdbe *v = (Vdbe *)pBlob.pStmt;
      int iDb = db.SchemaToIndex(pTab.Schema)

      v.AddOpList(OPEN_BLOB...)


      /* Configure the OP_Transaction */
      v.ChangeP1(0, iDb);
      v.ChangeP2(0, flags)

      /* Configure the OP_VerifyCookie */
      v.ChangeP1(1, iDb)
      v.ChangeP2(1, pTab.Schema.schema_cookie)
      v.ChangeP3(1, pTab.Schema.iGeneration)

      /* Make sure a mutex is held on the table to be accessed */
      sqlite3VdbeUsesBtree(v, iDb);

      /* Configure the OP_TableLock instruction */
      v.ChangeP1(2, iDb)
      v.ChangeP2(2, pTab.tnum)
      v.ChangeP3(2, flags)
      sqlite3VdbeChangeP4(v, 2, pTab.Name, P4_TRANSIENT);

      /* Remove either the OP_OpenWrite or OpenRead. Set the P2
      ** parameter of the other to pTab.tnum.  */
      v.ChangeToNoop(4 - flags)
      v.ChangeP2(3 + flags, pTab.tnum)
      v.ChangeP3(3 + flags, iDb)

      /* Configure the number of columns. Configure the cursor to
      ** think that the table has one more column than it really
      ** does. An OP_Column to retrieve this imaginary column will
      ** always return an SQL NULL. This is useful because it means
      ** we can invoke OP_Column to fill in the vdbe cursors type
      ** and offset cache without causing any IO.
      */
      sqlite3VdbeChangeP4(v, 3+flags, SQLITE_INT_TO_PTR(pTab.nCol+1),P4_INT32);
      v.ChangeP2(7, pTab.nCol)
      if( !db.mallocFailed ){
        pParse.nVar = 1;
        pParse.nMem = 1;
        pParse.nTab = 1;
        sqlite3VdbeMakeReady(v, pParse);
      }
    }

    pBlob.flags = flags;
    pBlob.iCol = iCol;
    pBlob.db = db;
    db.UnlockAll()
    if( db.mallocFailed ){
      goto blob_open_out;
    }
    sqlite3_bind_int64(pBlob.pStmt, 1, iRow);
    rc, zErr = pBlob.SeekToRow(iRow)
  } while( (++nAttempt)<5 && rc==SQLITE_SCHEMA );

blob_open_out:
  if( rc==SQLITE_OK && !db.mallocFailed ){
    *ppBlob = (sqlite3_blob *)pBlob;
  }else{
    if pBlob != nil && pBlob.pStmt != nil {
		(Vdbe *)(pBlob.pStmt).Finalize()
	}
    pBlob = nil
  }
  db.Error(rc, (zErr ? "%s" : 0), zErr);
  zErr = nil
  sqlite3StackFree(db, pParse);
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

//	Close a blob handle that was previously created using sqlite3_blob_open().
func (pBlob *sqlite3_blob) Close() (rc int) {
	if p := (Incrblob *)pBlob; p != nil {
		p.db.mutex.CriticalSection(func() {
			rc = sqlite3_finalize(p.pStmt)
		})
	}
	return
}

//	Perform a read or write operation on a blob
func (pBlob *sqlite3_blob) ReadWrite(z interface{}, n, iOffset int, xCall func(*btree.Cursor, uint32, uint32, interface{})) (rc int) {
	if p := (Incrblob *)(pBlob); p != nil {
		db := p.db
		db.mutex.CriticalSection(func() {
			switch v := (Vdbe*)(p.pStmt); {
			case n < 0 || iOffset < 0 || iOffset + n > p.nByte:
				//	Request is out of range. Return a transient error.
				rc = SQLITE_ERROR
				db.Error(SQLITE_ERROR, "")
			case v == nil:
				//	If there is no statement handle, then the blob-handle has already been invalidated. Return SQLITE_ABORT in this case.
				rc = SQLITE_ABORT
			default:
				//	Call either BtreeData() or BtreePutData(). If SQLITE_ABORT is returned, clean-up the statement handle.
				assert( db == v.db )
				p.pCsr.CriticalSection(func() {
					rc = xCall(p.pCsr, iOffset + p.iOffset, n, z)
				})
				if rc == SQLITE_ABORT {
					v.Finalize()
					p.pStmt = nil
				} else {
					db.errCode = rc
					v.rc = rc
				}
			}
			rc = db.ApiExit(rc)
		})
	} else {
		rc = SQLITE_MISUSE_BKPT
	}
	return
}

//	Read data from a blob handle.
func int sqlite3_blob_read(sqlite3_blob *pBlob, void *z, int n, int iOffset){
  return pBlob.ReadWrite(z, n, iOffset, sqlite3BtreeData)
}

//	Write data to a blob handle.
func int sqlite3_blob_write(sqlite3_blob *pBlob, const void *z, int n, int iOffset){
  return pBlob.ReadWrite(z, n, iOffset, sqlite3BtreePutData)
}

/*
** Query a blob handle for the size of the data.
**
** The Incrblob.nByte field is fixed for the lifetime of the Incrblob
** so no mutex is required for access.
*/
func int sqlite3_blob_bytes(sqlite3_blob *pBlob){
  Incrblob *p = (Incrblob *)pBlob;
  return (p && p.pStmt) ? p.nByte : 0;
}

//	Move an existing blob handle to point to a different row of the same database table.
//
//	If an error occurs, or if the specified row does not exist or does not contain a blob or text value, then an error code is returned and the
//	database handle error code and message set. If this happens, then all subsequent calls to sqlite3_blob_xxx() functions (except blob_close())
//	immediately return SQLITE_ABORT.

func int sqlite3_blob_reopen(sqlite3_blob *pBlob, int64 iRow) {
	rc	int
	zErr	string

	p := (Incrblob *)(pBlob)
	if( p==0 ) return SQLITE_MISUSE_BKPT;
	db := p.db;
	db.mutex.CriticalSection(func() {
		if( p.pStmt==0 ){
			//	If there is no statement handle, then the blob-handle has already been invalidated. Return SQLITE_ABORT in this case.
			rc = SQLITE_ABORT
		} else {
			if rc, zErr = p.SeekToRow(iRow); rc!=SQLITE_OK {
				db.Error(rc, (zErr ? "%s" : 0), zErr)
				zErr = nil
			}
			assert( rc!=SQLITE_SCHEMA );
		}
		rc = db.ApiExit(rc)
		assert( rc==SQLITE_OK || p.pStmt==0 );
	})
	return rc;
}
