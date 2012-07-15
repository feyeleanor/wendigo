/* This file contains the implementation of the sqlite3_backup_XXX() 
** API functions and the related features.
*/

/* Macro to find the minimum of two numeric values.
*/
#ifndef MIN
# define MIN(x,y) ((x)<(y)?(x):(y))
#endif

/*
** Structure allocated for each backup operation.
*/
struct sqlite3_backup {
  sqlite3* pDestDb;        /* Destination database handle */
  Btree *pDest;            /* Destination b-tree file */
  uint32 iDestSchema;         /* Original schema cookie in destination */
  int bDestLocked;         /* True once a write-transaction is open on pDest */

  PageNumber iNext;              /* Page number of the next source page to copy */
  sqlite3* pSrcDb;         /* Source database handle */
  Btree *pSrc;             /* Source b-tree file */

  int rc;                  /* Backup process error code */

  /* These two variables are set by every call to backup::step(). They are
  ** read by calls to backup_remaining() and backup_pagecount().
  */
  PageNumber nRemaining;         /* Number of pages left to copy */
  PageNumber nPagecount;         /* Total number of pages to copy */

  int isAttached;          /* True once backup has been registered with pager */
  sqlite3_backup *Next;   /* Next backup associated with source pager */
};

/*
** THREAD SAFETY NOTES:
**
**   Once it has been created using backup_init(), a single sqlite3_backup
**   structure may be accessed via two groups of thread-safe entry points:
**
**     * Via the sqlite3_backup_XXX() API function backup::step() and 
**       backup::finish(). Both these functions obtain the source database
**       handle mutex and the mutex associated with the source BtShared 
**       structure, in that order.
**
**     * Via the BackupUpdate() and BackupRestart() functions, which are
**       invoked by the pager layer to report various state changes in
**       the page cache associated with the source database. The mutex
**       associated with the source database BtShared structure will always 
**       be held when either of these functions are invoked.
**
**   The other sqlite3_backup_XXX() API functions, backup_remaining() and
**   backup_pagecount() are not thread-safe functions. If they are called
**   while some other thread is calling backup::step() or backup::finish(),
**   the values returned may be invalid. There is no way for a call to
**   BackupUpdate() or BackupRestart() to interfere with backup_remaining()
**   or backup_pagecount().
**
**   Depending on the SQLite configuration, the database handles and/or
**   the Btree objects may have their own mutexes that require locking.
**   Non-sharable Btrees (in-memory databases for example), do not have
**   associated mutexes.
*/

//	Return a pointer corresponding to database zDb (i.e. "main", "temp") in connection handle pDb. If such a database cannot be found, return a NULL pointer and write an error message to pErrorDb.
//	If the "temp" database is requested, it may need to be opened by this function. If an error occurs while doing so, return 0 and write an error message to pErrorDb.
func (db *sqlite3) findBtree(pDb *sqlite3, dbname string) *Btree {
	switch i := pDb.FindDbName(dbname); {
	case i == 1:
		rc := SQLITE_OK
		if pParse := sqlite3StackAllocZero(db, sizeof(*pParse)); pParse == nil {
			db.Error(SQLITE_NOMEM, "out of memory")
			rc = SQLITE_NOMEM
		} else {
			pParse.db = pDb
			if sqlite3OpenTempDatabase(pParse) {
				db.Error(pParse.rc, "%v", pParse.zErrMsg)
				rc = SQLITE_ERROR
			}
			pParse.zErrMsg = ""
			sqlite3StackFree(db, pParse)
		}
		if rc != SQLITE_OK {
			return nil
		}
	case i < 0:
		db.Error(SQLITE_ERROR, "unknown database %v", dbname)
		return nil
	}
	return pDb.Databases[i].pBt
}

//	Attempt to set the page size of the destination to match the page size of the source.
func (p *sqlite3_backup) setDestinationPageSize() int {
	return sqlite3BtreeSetPageSize(p.pDest, sqlite3BtreeGetPageSize(p.pSrc), -1, 0)
}

//	Create an sqlite3_backup process to copy the contents of zSrcDb from connection handle pSrcDb to zDestDb in pDestDb. If successful, return a pointer to the new sqlite3_backup object.
//	If an error occurs, NULL is returned and an error code and error message stored in database handle pDestDb.
func (destination *sqlite3) backup_init(zDestDb string, source *sqlite3, zSrcDb string) (backup *sqlite3_backup) {
	//	Lock the source database handle. The destination database handle is not locked in this routine, but it is locked in backup::step(). The user is required to ensure that no other thread accesses the destination handle for the duration of the backup operation. Any attempt to use the destination database connection while a backup is in progress may cause a malfunction or a deadlock.
	source.mutex.CriticalSection(func() {
		destination.mutex.CriticalSection(func() {
			if source == destination {
				destination.Error(SQLITE_ERROR, "source and destination must be distinct")
			} else {
				backup = &sqlite3_backup{
					pSrc:		destination.findBtree(source, zSrcDb),
					pDest:		destination.findBtree(destination, zDestDb),
					pDestDb:	destination,
					pSrcDb:		source,
					iNext:		1,
				}

				if backup.pSrc == nil || backup.pDest == nil || backup.setDestinationPageSize() == SQLITE_NOMEM {
					//	One (or both) of the named databases did not exist or an OOM error was hit. The error has already been written into the pDestDb handle. All that is left to do here is free the sqlite3_backup structure.
					backup = nil
				}
				if backup != nil {
					backup.pSrc.nBackup++
				}
			}
		})
	})
	return
}

/*
** Argument rc is an SQLite error code. Return true if this error is 
** considered fatal if encountered during a backup operation. All errors
** are considered fatal except for SQLITE_BUSY and SQLITE_LOCKED.
*/
static int isFatalError(int rc){
  return (rc!=SQLITE_OK && rc!=SQLITE_BUSY && rc!=SQLITE_LOCKED);
}

/*
** Parameter zSrcData points to a buffer containing the data for 
** page iSrcPg from the source database. Copy this data into the 
** destination database.
*/
static int backupOnePage(sqlite3_backup *p, PageNumber iSrcPg, const byte *zSrcData){
  Pager * const pDestPager = p.pDest.Pager()
  const int nSrcPgsz = sqlite3BtreeGetPageSize(p.pSrc);
  int nDestPgsz = sqlite3BtreeGetPageSize(p.pDest);
  const int nCopy = MIN(nSrcPgsz, nDestPgsz);
  const int64 iEnd = (int64)iSrcPg*(int64)nSrcPgsz;
  int nSrcReserve = sqlite3BtreeGetReserve(p.pSrc);
  int nDestReserve = sqlite3BtreeGetReserve(p.pDest);

  int rc = SQLITE_OK;
  int64 iOff;

  assert( p.bDestLocked );
  assert( !isFatalError(p.rc) );
  assert( iSrcPg!=PAGER_MJ_PGNO(p.pSrc.pBt) );
  assert( zSrcData );

  /* Catch the case where the destination is an in-memory database and the
  ** page sizes of the source and destination differ. 
  */
  if( nSrcPgsz!=nDestPgsz && sqlite3PagerIsMemdb(pDestPager) ){
    rc = SQLITE_READONLY;
  }

  /* Backup is not possible if the page size of the destination is changing
  ** and a codec is in use.
  */
  if( nSrcPgsz!=nDestPgsz && sqlite3PagerGetCodec(pDestPager)!=0 ){
    rc = SQLITE_READONLY;
  }

  /* Backup is not possible if the number of bytes of reserve space differ
  ** between source and destination.  If there is a difference, try to
  ** fix the destination to agree with the source.  If that is not possible,
  ** then the backup cannot proceed.
  */
  if( nSrcReserve!=nDestReserve ){
    uint32 newPgsz = nSrcPgsz;
    rc = sqlite3PagerSetPagesize(pDestPager, &newPgsz, nSrcReserve);
    if( rc==SQLITE_OK && newPgsz!=nSrcPgsz ) rc = SQLITE_READONLY;
  }

	//	This loop runs once for each destination page spanned by the source page. For each iteration, variable iOff is set to the byte offset of the destination page.
	for iOff = iEnd - int64(nSrcPgsz); rc == SQLITE_OK && iOff < iEnd; iOff += nDestPgsz) {
		DbPage *pDestPg = 0
		iDest := PageNumber((iOff / nDestPgsz) + 1)
		if iDest == PAGER_MJ_PGNO(p.pDest.pBt) {
			continue
		}
		if pDestPg, rc = pDestPager.Acquire(iDest, false); rc == SQLITE_OK {
			if rc = sqlite3PagerWrite(pDestPg) == SQLITE_OK {
				zIn := zSrcData[iOff % nSrcPgsz]
				zDestData := pDestPg.GetData()
				zOut := zDestData[iOff % nDestPgsz]

				//	Copy the data from the source page into the destination page. Then clear the Btree layer MemoryPage.isInit flag. Both this module
				//	and the pager code use this trick (clearing the first byte of the page 'extra' space to invalidate the Btree layers
				//	cached parse of the page). MemoryPage.isInit is marked "MUST BE FIRST" for this purpose.
				memcpy(zOut, zIn, nCopy)
				pDestPg.GetExtra()[0] = 0
			}
		}
    	sqlite3PagerUnref(pDestPg)
  	}
	return rc
}

/*
** If pFile is currently larger than iSize bytes, then truncate it to
** exactly iSize bytes. If pFile is not larger than iSize bytes, then
** this function is a no-op.
**
** Return SQLITE_OK if everything is successful, or an SQLite error 
** code if an error occurs.
*/
static int backupTruncateFile(sqlite3_file *pFile, int64 iSize){
  int64 iCurrent;
  int rc = sqlite3OsFileSize(pFile, &iCurrent);
  if( rc==SQLITE_OK && iCurrent>iSize ){
    rc = sqlite3OsTruncate(pFile, iSize);
  }
  return rc;
}

/*
** Register this backup object with the associated source pager for
** callbacks when pages are changed or the cache invalidated.
*/
static void attachBackupObject(sqlite3_backup *p){
  sqlite3_backup **pp;
  pp = sqlite3PagerBackupPtr(p.pSrc.Pager())
  p.Next = *pp;
  *pp = p;
  p.isAttached = 1;
}

//	Copy nPage pages from the source b-tree to the destination.
func (p *sqlite3_backup) step(pages int) (rc int) {
	int destMode;       /* Destination journal mode */
	pgszSrc := 0			//	Source page size
	pgszDest := 0			//	Destination page size

	p.pSrcDb.mutex.CriticalSection(func() {
		p.pSrc.Lock()
		if p.pDestDb != nil {
			p.pDestDb.mutex.Lock()
		}

		rc = p.rc
		if !isFatalError(rc) {
			pSrcPager := p.pSrc.Pager()     /* Source pager */
			pDestPager := p.pDest.Pager()   /* Dest pager */
			int ii;                            /* Iterator variable */
			nSrcPage := -1                 /* Size of source db in pages */
			bCloseTrans := false               /* True if src db requires unlocking */

			//	If the source pager is currently in a write-transaction, return SQLITE_BUSY immediately.
			if p.pDestDb && p.pSrc.pBt.inTransaction == TRANS_WRITE {
				rc = SQLITE_BUSY
			} else {
				rc = SQLITE_OK
			}

			//	Lock the destination database, if it is not locked already.
			if rc == SQLITE_OK && !p.bDestLocked {
				if rc = sqlite3BtreeBeginTrans(p.pDest, 2)); rc == SQLITE_OK {
					p.bDestLocked = true
					sqlite3BtreeGetMeta(p.pDest, BTREE_SCHEMA_VERSION, &p.iDestSchema)
				}
			}

			//	If there is no open read-transaction on the source database, open one now. If a transaction is opened here, then it will be closed before this function exits.
			if rc == SQLITE_OK && !sqlite3BtreeIsInReadTrans(p.pSrc) {
				rc = sqlite3BtreeBeginTrans(p.pSrc, 0)
				bCloseTrans = true
			}

			//	Do not allow backup if the destination database is in WAL mode and the page sizes are different between source and destination
			pgszSrc = sqlite3BtreeGetPageSize(p.pSrc)
			pgszDest = sqlite3BtreeGetPageSize(p.pDest)
			destMode = sqlite3PagerGetJournalMode(p.pDest.Pager())
			if rc == SQLITE_OK && destMode == PAGER_JOURNALMODE_WAL && pgszSrc != pgszDest {
				rc = SQLITE_READONLY
			}
  
			//	Now that there is a read-lock on the source database, query the source pager for the number of pages in the database.
			nSrcPage = int(sqlite3BtreeLastPage(p.pSrc))
			assert( nSrcPage >= 0 )
			for(ii=0; (nPage<0 || ii<nPage) && p.iNext<=(PageNumber)nSrcPage && !rc; ii++){
				iSrcPg := p.iNext                 /* Source page number */
				if iSrcPg != PAGER_MJ_PGNO(p.pSrc.pBt) {
					DbPage *pSrcPg;                             /* Source page object */
					if pSrcPg, rc = pSrcPager.Acquire(iSrcPg, false); rc == SQLITE_OK {
						rc = backupOnePage(p, iSrcPg, pSrcPg.GetData())
						sqlite3PagerUnref(pSrcPg)
					}
				}
				p.iNext++
			}
			if rc == SQLITE_OK {
				p.nPagecount = nSrcPage
				p.nRemaining = nSrcPage + 1 - p.iNext
				if p.iNext > PageNumber(nSrcPage) {
					rc = SQLITE_DONE
				} else if !p.isAttached {
					attachBackupObject(p)
				}
			}
  
			//	Update the schema version field in the destination database. This is to make sure that the schema-version really does change in the case where the source and destination databases have the same schema version.
			if rc == SQLITE_DONE {
				if rc = sqlite3BtreeUpdateMeta(p.pDest, 1, p.iDestSchema + 1); rc == SQLITE_OK {
					if p.pDestDb != nil {
						p.pDestDb.ResetInternalSchema(-1)
					}
					if destMode == PAGER_JOURNALMODE_WAL {
						rc = sqlite3BtreeSetVersion(p.pDest, 2)
					}
				}
				if rc == SQLITE_OK {
					int nDestTruncate
					//	Set nDestTruncate to the final number of pages in the destination database. The complication here is that the destination page size may be different to the source page size. 
					//	If the source page size is smaller than the destination page size, round up. In this case the call to sqlite3OsTruncate() below will fix the size of the file. However it is important to call sqlite3PagerTruncateImage() here so that any pages in the destination file that lie beyond the nDestTruncate page mark are journalled by PagerCommitPhaseOne() before they are destroyed by the file truncation.
					assert( pgszSrc == sqlite3BtreeGetPageSize(p.pSrc) )
					assert( pgszDest == sqlite3BtreeGetPageSize(p.pDest) )
					if pgszSrc < pgszDest {
						int ratio = pgszDest / pgszSrc
						nDestTruncate = (nSrcPage + ratio - 1) / ratio
						if nDestTruncate == int(PAGER_MJ_PGNO(p.pDest.pBt)) {
							nDestTruncate--
						}
					} else {
						nDestTruncate = nSrcPage * (pgszSrc /p gszDest)
					}
					sqlite3PagerTruncateImage(pDestPager, nDestTruncate)

					if pgszSrc < pgszDest {
						//	If the source page-size is smaller than the destination page-size, two extra things may need to happen:
						//		* The destination may need to be truncated, and
						//		* Data stored on the pages immediately following the pending-byte page in the source database may need to be copied into the destination database.
						const int64 iSize = (int64)pgszSrc * (int64)nSrcPage
						sqlite3_file * const pFile = sqlite3PagerFile(pDestPager)
						int64 iOff
						int64 iEnd

						assert( pFile )
						assert( (int64)nDestTruncate * (int64)pgszDest >= iSize || (nDestTruncate==(int)(PAGER_MJ_PGNO(p.pDest.pBt) - 1) && iSize >= PENDING_BYTE && iSize <= PENDING_BYTE + pgszDest) )

						//	This call ensures that all data required to recreate the original database has been stored in the journal for pDestPager and the journal synced to disk. So at this point we may safely modify the database file in any way, knowing that if a power failure occurs, the original database will be reconstructed from the journal file.
						rc = sqlite3PagerCommitPhaseOne(pDestPager, 0, 1)

						//	Write the extra pages and truncate the database file as required
						iEnd = MIN(PENDING_BYTE + pgszDest, iSize)
						for iOff = PENDING_BYTE + pgszSrc; rc == SQLITE_OK && iOff < iEnd; iOff += pgszSrc {
							PgHdr *pSrcPg = 0
							const PageNumber iSrcPg = PageNumber((iOff / pgszSrc) + 1)
							if pSrcPg, rc = pSrcPager.Acquire(iSrcPg, false); rc == SQLITE_OK {
								rc = sqlite3OsWrite(pFile, pSrcPg.GetData(), pgszSrc, iOff)
							}
							sqlite3PagerUnref(pSrcPg)
						}
						if rc == SQLITE_OK {
							rc = backupTruncateFile(pFile, iSize)
						}

						//	Sync the database file to disk.
						if rc == SQLITE_OK {
							rc = sqlite3PagerSync(pDestPager)
						}
					} else {
						rc = sqlite3PagerCommitPhaseOne(pDestPager, 0, 0)
					}
    
					//	Finish committing the transaction to the destination database.
					if rc == SQLITE_OK {
						if rc = sqlite3BtreeCommitPhaseTwo(p.pDest, 0); rc == SQLITE_OK {
							rc = SQLITE_DONE
						}
					}
				}
			}
  
			//	If bCloseTrans is true, then this function opened a read transaction on the source database. Close the read transaction here. There is no need to check the return values of the btree methods here, as "committing" a read-only transaction cannot fail.
			if bCloseTrans {
				sqlite3BtreeCommitPhaseOne(p.pSrc, 0)
				sqlite3BtreeCommitPhaseTwo(p.pSrc, 0)
			}
  
			if rc == SQLITE_IOERR_NOMEM {
				rc = SQLITE_NOMEM
			}
			p.rc = rc
		}
		if p.pDestDb {
			p.pDestDb.mutex.Unlock()
		}
		p.pSrc.Unlock()
	})
	return
}

//	Release all resources associated with an sqlite3_backup* handle.
func (p *sqlite3_backup) finish() (rc int) {
	sqlite3_backup **pp;                 /* Ptr to head of pagers backup list */

	//	Enter the mutexes
	if p != nil {
		p.pSrcDb.mutex.CriticalSection(func() {
			p.pSrc.Lock()
			if p.pDestDb {
				p.pDestDb.mutex.Lock()
			}

			//	Detach this backup from the source pager.
			if p.pDestDb {
				p.pSrc.nBackup--
			}
			if p.isAttached {
				pp = sqlite3PagerBackupPtr(p.pSrc.Pager())
				while *pp != p {
					pp = &(*pp).Next
				}
				*pp = p.Next
			}

			//	If a transaction is still open on the Btree, roll it back.
			p.pDest.Rollback(SQLITE_OK)

			//	Set the error code of the destination database handle.
			rc = (p.rc==SQLITE_DONE) ? SQLITE_OK : p.rc
			p.pDestDb.Error(rc, "")

			//	Exit the mutexes and free the backup context structure.
			if p.pDestDb {
				p.pDestDb.mutex.Unlock()
			}
			p.pSrc.Unlock()
			if p.pDestDb {
				//	EVIDENCE-OF: R-64852-21591 The sqlite3_backup object is created by a call to backup_init() and is destroyed by a call to backup::finish().
				p = nil
			}
		})
	}
	return
}

/*
** Return the number of pages still to be backed up as of the most recent
** call to backup::step().
*/
func int sqlite3_backup_remaining(sqlite3_backup *p){
  return p.nRemaining;
}

/*
** Return the total number of pages in the source database as of the most 
** recent call to backup::step().
*/
func int sqlite3_backup_pagecount(sqlite3_backup *p){
  return p.nPagecount;
}

/*
** This function is called after the contents of page iPage of the
** source database have been modified. If page iPage has already been 
** copied into the destination database, then the data written to the
** destination is now invalidated. The destination copy of iPage needs
** to be updated with the new data before the backup operation is
** complete.
**
** It is assumed that the mutex associated with the BtShared object
** corresponding to the source database is held when this function is
** called.
*/
 void sqlite3BackupUpdate(sqlite3_backup *pBackup, PageNumber iPage, const byte *aData){
  sqlite3_backup *p;                   /* Iterator variable */
  for(p=pBackup; p; p=p.Next){
    if( !isFatalError(p.rc) && iPage<p.iNext ){
      /* The backup process p has already copied page iPage. But now it
      ** has been modified by a transaction on the source pager. Copy
      ** the new data into the backup.
      */
      int rc;
      assert( p.pDestDb );
      p.pDestDb.mutex.Lock()
      rc = backupOnePage(p, iPage, aData);
      p.pDestDb.mutex.Unlock()
      assert( rc!=SQLITE_BUSY && rc!=SQLITE_LOCKED );
      if( rc!=SQLITE_OK ){
        p.rc = rc;
      }
    }
  }
}

/*
** Restart the backup process. This is called when the pager layer
** detects that the database has been modified by an external database
** connection. In this case there is no way of knowing which of the
** pages that have been copied into the destination database are still 
** valid and which are not, so the entire process needs to be restarted.
**
** It is assumed that the mutex associated with the BtShared object
** corresponding to the source database is held when this function is
** called.
*/
 void sqlite3BackupRestart(sqlite3_backup *pBackup){
  sqlite3_backup *p;                   /* Iterator variable */
  for(p=pBackup; p; p=p.Next){
    p.iNext = 1;
  }
}

#ifndef SQLITE_OMIT_VACUUM
/*
** Copy the complete content of pBtFrom into pBtTo.  A transaction
** must be active for both files.
**
** The size of file pTo may be reduced by this operation. If anything 
** goes wrong, the transaction on pTo is rolled back. If successful, the 
** transaction is committed before returning.
*/
 int sqlite3BtreeCopyFile(Btree *pTo, Btree *pFrom){
  int rc;
  sqlite3_file *pFd;              /* File descriptor for database pTo */
  sqlite3_backup b;
  pTo.Lock()
  pFrom.Lock()

  assert( pTo.IsInTrans() )
  pFd = sqlite3PagerFile(pTo.Pager());
  if( pFd.pMethods ){
    int64 nByte = sqlite3BtreeGetPageSize(pFrom)*(int64)sqlite3BtreeLastPage(pFrom);
    rc = sqlite3OsFileControl(pFd, SQLITE_FCNTL_OVERWRITE, &nByte);
    if( rc==SQLITE_NOTFOUND ) rc = SQLITE_OK;
    if( rc ) goto copy_finished;
  }

  /* Set up an sqlite3_backup object. sqlite3_backup.pDestDb must be set
  ** to 0. This is used by the implementations of backup::step()
  ** and backup::finish() to detect that they are being called
  ** from this function, not directly by the user.
  */
  memset(&b, 0, sizeof(b));
  b.pSrcDb = pFrom.db;
  b.pSrc = pFrom;
  b.pDest = pTo;
  b.iNext = 1;

  /* 0x7FFFFFFF is the hard limit for the number of pages in a database
  ** file. By passing this as the number of pages to copy to
  ** backup::step(), we can guarantee that the copy finishes 
  ** within a single call (unless an error occurs). The assert() statement
  ** checks this assumption - (p.rc) should be set to either SQLITE_DONE 
  ** or an error code.
  */
  b.step(0x7FFFFFFF)
  assert( b.rc!=SQLITE_OK );
  rc = b.finish()
  if( rc==SQLITE_OK ){
    pTo.pBt.Flags &= ~BTS_PAGESIZE_FIXED;
  }else{
    sqlite3PagerClearCache(b.pDest.Pager());
  }

  assert( !pTo.IsInTrans() )
copy_finished:
  pFrom.Unlock()
  pTo.Unlock()
  return rc;
}
#endif /* SQLITE_OMIT_VACUUM */
