/* This file contains code use to implement APIs that are part of the
** VDBE.
*/

/*
** Check on a Vdbe to make sure it has not been finalized.  Log
** an error and return true if it has been finalized (or is otherwise
** invalid).  Return false if it is ok.
*/
static int vdbeSafety(Vdbe *p){
  if( p.db==0 ){
    sqlite3_log(SQLITE_MISUSE, "API called with finalized prepared statement");
    return 1;
  }else{
    return 0;
  }
}
static int vdbeSafetyNotNull(Vdbe *p){
  if( p==0 ){
    sqlite3_log(SQLITE_MISUSE, "API called with NULL prepared statement");
    return 1;
  }else{
    return vdbeSafety(p);
  }
}

/*
** The following routine destroys a virtual machine that is created by
** the sqlite3_compile() routine. The integer returned is an SQLITE_
** success/failure code that describes the result of executing the virtual
** machine.
**
** This routine sets the error code and string returned by
** sqlite3_errcode(), sqlite3_errmsg() and sqlite3_errmsg16().
*/
func int sqlite3_finalize(sqlite3_stmt *pStmt){
  int rc;
  if( pStmt==0 ){
    /* IMPLEMENTATION-OF: R-57228-12904 Invoking sqlite3_finalize() on a NULL
    ** pointer is a harmless no-op. */
    rc = SQLITE_OK;
  }else{
    Vdbe *v = (Vdbe*)pStmt;
    sqlite3 *db = v.db;
    sqlite3_mutex *mutex;
    if( vdbeSafety(v) ) return SQLITE_MISUSE_BKPT;
    mutex = v.db.mutex;
    mutex.Lock()
    rc = sqlite3VdbeFinalize(v);
    rc = db.ApiExit(rc)
    mutex.Unlock()
  }
  return rc;
}

/*
** Terminate the current execution of an SQL statement and reset it
** back to its starting state so that it can be reused. A success code from
** the prior execution is returned.
**
** This routine sets the error code and string returned by
** sqlite3_errcode(), sqlite3_errmsg() and sqlite3_errmsg16().
*/
func int sqlite3_reset(sqlite3_stmt *pStmt){
  int rc;
  if( pStmt==0 ){
    rc = SQLITE_OK;
  }else{
    Vdbe *v = (Vdbe*)pStmt;
    v.db.mutex.Lock()
    rc = v.Reset()
    v.Rewind()
    assert( (rc & (v.db.errMask))==rc );
    rc = v.db.ApiExit(rc)
    v.db.mutex.Unlock()
  }
  return rc;
}

/*
** Set all the parameters in the compiled SQL statement to NULL.
*/
func int sqlite3_clear_bindings(sqlite3_stmt *pStmt){
  int i;
  int rc = SQLITE_OK;
  Vdbe *p = (Vdbe*)pStmt;
  sqlite3_mutex *mutex = ((Vdbe*)pStmt).db.mutex;
  mutex.Lock()
  for(i=0; i<p.nVar; i++){
    p.aVar[i].Release()
    p.aVar[i].flags = MEM_Null;
  }
  if( p.isPrepareV2 && p.expmask ){
    p.expired = 1;
  }
  mutex.Unlock()
  return rc;
}


/**************************** sqlite3_value_  *******************************
** The following routines extract information from a Mem or sqlite3_value
** structure.
*/
func const void *sqlite3_value_blob(sqlite3_value *pVal){
  Mem *p = (Mem*)pVal;
  if( p.flags & (MEM_Blob|MEM_Str) ){
    p.ExpandBlob()
    p.flags &= ~MEM_Str;
    p.flags |= MEM_Blob;
    return p.n ? p.z : 0;
  }else{
    return sqlite3_value_text(pVal);
  }
}
func int sqlite3_value_bytes(sqlite3_value *pVal){
  return pVal.ValueBytes(SQLITE_UTF8)
}
func int sqlite3_value_bytes16(sqlite3_value *pVal){
  return pVal.ValueBytes(SQLITE_UTF16NATIVE)
}
func double sqlite3_value_double(sqlite3_value *pVal){
  return sqlite3VdbeRealValue((Mem*)pVal);
}
func int sqlite3_value_int(sqlite3_value *pVal){
  return (int)sqlite3VdbeIntValue((Mem*)pVal);
}
func sqlite_int64 sqlite3_value_int64(sqlite3_value *pVal){
  return sqlite3VdbeIntValue((Mem*)pVal);
}
func const unsigned char *sqlite3_value_text(sqlite3_value *pVal){
  return (const unsigned char *)sqlite3ValueText(pVal, SQLITE_UTF8);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_value_text16(sqlite3_value* pVal){
  return sqlite3ValueText(pVal, SQLITE_UTF16NATIVE);
}
func const void *sqlite3_value_text16be(sqlite3_value *pVal){
  return sqlite3ValueText(pVal, SQLITE_UTF16BE);
}
func const void *sqlite3_value_text16le(sqlite3_value *pVal){
  return sqlite3ValueText(pVal, SQLITE_UTF16LE);
}
#endif /* SQLITE_OMIT_UTF16 */
func int sqlite3_value_type(sqlite3_value* pVal){
  return pVal.Type;
}

/**************************** sqlite3_result_  *******************************
** The following routines are used by user-defined functions to specify
** the function result.
**
** The setStrOrError() funtion calls Mem::SetStr() to store the
** result as a string or blob but if the string or blob is too large, it
** then sets the error code to SQLITE_TOOBIG
*/
static void setResultStrOrError(
  sqlite3_context *pCtx,  /* Function context */
  const char *z,          /* String pointer */
  int n,                  /* Bytes in string, or negative */
  byte enc,                 /* Encoding of z.  0 for BLOBs */
  void (*xDel)(void*)     /* Destructor function */
){
  if pCtx.s.SetStr(z, enc, xDel) == SQLITE_TOOBIG {
    sqlite3_result_error_toobig(pCtx);
  }
}
func void sqlite3_result_blob(
  sqlite3_context *pCtx, 
  const void *z, 
  int n, 
  void (*xDel)(void *)
){
  assert( n>=0 );
  setResultStrOrError(pCtx, z, n, 0, xDel);
}
func void sqlite3_result_double(sqlite3_context *pCtx, double rVal){
  sqlite3VdbeMemSetDouble(&pCtx.s, rVal);
}
func void sqlite3_result_error(sqlite3_context *pCtx, const char *z, int n){
  pCtx.isError = SQLITE_ERROR;
  pCtx.s.SetStr(z, SQLITE_UTF8, SQLITE_TRANSIENT)
}
#ifndef SQLITE_OMIT_UTF16
func void sqlite3_result_error16(sqlite3_context *pCtx, const void *z, int n){
  pCtx.isError = SQLITE_ERROR;
  pCtx.s.SetStr(z, SQLITE_UTF16NATIVE, SQLITE_TRANSIENT)
}
#endif
func void sqlite3_result_int(sqlite3_context *pCtx, int iVal){
	pCtx.s.SetInt64(int64(iVal))
}
func void sqlite3_result_int64(sqlite3_context *pCtx, int64 iVal){
	pCtx.s.SetInt64(iVal)
}
func void sqlite3_result_null(sqlite3_context *pCtx){
  pCtx.s.SetNull()
}
func void sqlite3_result_text(
  sqlite3_context *pCtx, 
  const char *z, 
  int n,
  void (*xDel)(void *)
){
  setResultStrOrError(pCtx, z, n, SQLITE_UTF8, xDel);
}
#ifndef SQLITE_OMIT_UTF16
func void sqlite3_result_text16(
  sqlite3_context *pCtx, 
  const void *z, 
  int n, 
  void (*xDel)(void *)
){
  setResultStrOrError(pCtx, z, n, SQLITE_UTF16NATIVE, xDel);
}
func void sqlite3_result_text16be(
  sqlite3_context *pCtx, 
  const void *z, 
  int n, 
  void (*xDel)(void *)
){
  setResultStrOrError(pCtx, z, n, SQLITE_UTF16BE, xDel);
}
func void sqlite3_result_text16le(
  sqlite3_context *pCtx, 
  const void *z, 
  int n, 
  void (*xDel)(void *)
){
  setResultStrOrError(pCtx, z, n, SQLITE_UTF16LE, xDel);
}
#endif /* SQLITE_OMIT_UTF16 */
func void sqlite3_result_value(sqlite3_context *pCtx, sqlite3_value *pValue){
  sqlite3VdbeMemCopy(&pCtx.s, pValue);
}
func (pCtx *sqlite3_context) result_zeroblob(n Zeroes) {
	pCtx.s.SetZeroBlob(n)
}
func void sqlite3_result_error_code(sqlite3_context *pCtx, int errCode){
  pCtx.isError = errCode;
  if( pCtx.s.flags & MEM_Null ){
    &pCtx.s.SetStr(sqlite3ErrStr(errCode), SQLITE_UTF8, SQLITE_STATIC)
  }
}

/* Force an SQLITE_TOOBIG error. */
func void sqlite3_result_error_toobig(sqlite3_context *pCtx){
  pCtx.isError = SQLITE_TOOBIG;
  pCtx.s.SetStr("string or blob too big", SQLITE_UTF8, SQLITE_STATIC)
}

/* An SQLITE_NOMEM error. */
func void sqlite3_result_error_nomem(sqlite3_context *pCtx){
  pCtx.s.SetNull()
  pCtx.isError = SQLITE_NOMEM;
  pCtx.s.db.mallocFailed = true
}

//	This function is called after a transaction has been committed. It invokes callbacks registered with sqlite3_wal_hook() as required.
func (db *sqlite3) doWalCallbacks() (rc int) {
	rc = SQLITE_OK
	for _, database := range db.Databases {
		if pBt := database.pBt; pBt != nil {
			nEntry := sqlite3PagerWalCallback(pBt.Pager())
			if db.xWalCallback != nil && nEntry > 0 && rc == SQLITE_OK {
				rc = db.xWalCallback(db.pWalArg, db, database.Name, nEntry)
			}
		}
	}
	return
}

/*
** Execute the statement pStmt, either until a row of data is ready, the
** statement is completely executed or an error occurs.
**
** This routine implements the bulk of the logic behind the sqlite_step()
** API.  The only thing omitted is the automatic recompile if a 
** schema change has occurred.  That detail is handled by the
** outer sqlite3_step() wrapper procedure.
*/
static int sqlite3Step(Vdbe *p){
  sqlite3 *db;
  int rc;

  assert(p);
  if( p.magic!=VDBE_MAGIC_RUN ){
    /* We used to require that sqlite3_reset() be called before retrying
    ** sqlite3_step() after any error or after SQLITE_DONE.  But beginning
    ** with version 3.7.0, we changed this so that sqlite3_reset() would
    ** be called automatically instead of throwing the SQLITE_MISUSE error.
    ** This "automatic-reset" change is not technically an incompatibility, 
    ** since any application that receives an SQLITE_MISUSE is broken by
    ** definition.
    **
    ** Nevertheless, some published applications that were originally written
    ** for version 3.6.23 or earlier do in fact depend on SQLITE_MISUSE 
    ** returns, and those were broken by the automatic-reset change.  As a
    ** a work-around, the SQLITE_OMIT_AUTORESET compile-time restores the
    ** legacy behavior of returning SQLITE_MISUSE for cases where the 
    ** previous sqlite3_step() returned something other than a SQLITE_LOCKED
    ** or SQLITE_BUSY error.
    */
#ifdef SQLITE_OMIT_AUTORESET
    if( p.rc==SQLITE_BUSY || p.rc==SQLITE_LOCKED ){
      sqlite3_reset((sqlite3_stmt*)p);
    }else{
      return SQLITE_MISUSE_BKPT;
    }
#else
    sqlite3_reset((sqlite3_stmt*)p);
#endif
  }

  /* Check that malloc() has not failed. If it has, return early. */
  db = p.db;
  if( db.mallocFailed ){
    p.rc = SQLITE_NOMEM;
    return SQLITE_NOMEM;
  }

  if( p.pc<=0 && p.expired ){
    p.rc = SQLITE_SCHEMA;
    rc = SQLITE_ERROR;
    goto end_of_step;
  }
  if( p.pc<0 ){
    /* If there are no other statements currently running, then
    ** reset the interrupt flag.  This prevents a call to sqlite3_interrupt
    ** from interrupting a statement that has not yet started.
    */
    if( db.activeVdbeCnt==0 ){
      db.u1.isInterrupted = 0;
    }

    assert( db.writeVdbeCnt>0 || db.autoCommit==0 || db.nDeferredCons==0 );

#ifndef SQLITE_OMIT_TRACE
    if( db.xProfile && !db.init.busy ){
      sqlite3OsCurrentTimeInt64(db.pVfs, &p.startTime);
    }
#endif

    db.activeVdbeCnt++;
    if( p.readOnly==0 ) db.writeVdbeCnt++;
    p.pc = 0;
  }
#ifndef SQLITE_OMIT_EXPLAIN
	if p.explain != 0 {
		rc = p.List()
	} else
#endif /* SQLITE_OMIT_EXPLAIN */
  {
    db.vdbeExecCnt++;
    rc = sqlite3VdbeExec(p);
    db.vdbeExecCnt--;
  }

#ifndef SQLITE_OMIT_TRACE
  /* Invoke the profile callback if there is one
  */
  if( rc!=SQLITE_ROW && db.xProfile && !db.init.busy && p.zSql ){
    int64 iNow;
    sqlite3OsCurrentTimeInt64(db.pVfs, &iNow);
    db.xProfile(db.pProfileArg, p.zSql, (iNow - p.startTime)*1000000);
  }
#endif

  if( rc==SQLITE_DONE ){
    assert( p.rc==SQLITE_OK );
    p.rc = db.doWalCallbacks()
    if( p.rc!=SQLITE_OK ){
      rc = SQLITE_ERROR;
    }
  }

  db.errCode = rc;
  if p.db.ApiExit(p.rc) == SQLITE_NOMEM {
    p.rc = SQLITE_NOMEM
  }
end_of_step:
  /* At this point local variable rc holds the value that should be 
  ** returned if this statement was compiled using the legacy 
  ** sqlite3_prepare() interface. According to the docs, this can only
  ** be one of the values in the first assert() below. Variable p.rc 
  ** contains the value that would be returned if sqlite3_finalize() 
  ** were called on statement p.
  */
  assert( rc==SQLITE_ROW  || rc==SQLITE_DONE   || rc==SQLITE_ERROR 
       || rc==SQLITE_BUSY || rc==SQLITE_MISUSE
  );
  assert( p.rc!=SQLITE_ROW && p.rc!=SQLITE_DONE );
  if( p.isPrepareV2 && rc!=SQLITE_ROW && rc!=SQLITE_DONE ){
    /* If this statement was prepared using sqlite3_prepare_v2(), and an
    ** error has occured, then return the error code in p.rc to the
    ** caller. Set the error code in the database handle to the same value.
    */ 
    rc = sqlite3VdbeTransferError(p);
  }
  return (rc&db.errMask);
}

/*
** The maximum number of times that a statement will try to reparse
** itself before giving up and returning SQLITE_SCHEMA.
*/
#ifndef SQLITE_MAX_SCHEMA_RETRY
# define SQLITE_MAX_SCHEMA_RETRY 5
#endif

/*
** This is the top-level implementation of sqlite3_step().  Call
** sqlite3Step() to do most of the work.  If a schema error occurs,
** call sqlite3Reprepare() and try again.
*/
func int sqlite3_step(sqlite3_stmt *pStmt){
  int rc = SQLITE_OK;      /* Result from sqlite3Step() */
  int rc2 = SQLITE_OK;     /* Result from sqlite3Reprepare() */
  Vdbe *v = (Vdbe*)pStmt;  /* the prepared statement */
  int cnt = 0;             /* Counter to prevent infinite loop of reprepares */
  sqlite3 *db;             /* The database connection */

  if( vdbeSafetyNotNull(v) ){
    return SQLITE_MISUSE_BKPT;
  }
  db = v.db;
  db.mutex.Lock()
  while( (rc = sqlite3Step(v))==SQLITE_SCHEMA
         && cnt++ < SQLITE_MAX_SCHEMA_RETRY
         && (rc2 = rc = sqlite3Reprepare(v))==SQLITE_OK ){
    sqlite3_reset(pStmt);
    assert( v.expired==0 );
  }
  if( rc2!=SQLITE_OK && v.isPrepareV2 && db.pErr ){
    /* This case occurs after failing to recompile an sql statement. 
    ** The error message from the SQL compiler has already been loaded 
    ** into the database handle. This block copies the error message 
    ** from the database handle into the statement and sets the statement
    ** program counter to 0 to ensure that when the statement is 
    ** finalized or reset the parser error message is available via
    ** sqlite3_errmsg() and sqlite3_errcode().
    */
    const char *zErr = (const char *)sqlite3_value_text(db.pErr); 
    v.zErrMsg = sqlite3DbStrDup(db, zErr);
    v.rc = rc2;
  }
  rc = db.ApiExit(rc)
  db.mutex.Unlock()
  return rc;
}

/*
** Extract the user data from a sqlite3_context structure and return a
** pointer to it.
*/
func void *sqlite3_user_data(sqlite3_context *p){
  assert( p && p.pFunc );
  return p.pFunc.pUserData;
}

/*
** Extract the user data from a sqlite3_context structure and return a
** pointer to it.
**
** IMPLEMENTATION-OF: R-46798-50301 The sqlite3_context_db_handle() interface
** returns a copy of the pointer to the database connection (the 1st
** parameter) of the sqlite3_create_function() and
** sqlite3_create_function16() routines that originally registered the
** application defined function.
*/
func sqlite3 *sqlite3_context_db_handle(sqlite3_context *p){
  assert( p && p.pFunc );
  return p.s.db;
}

/*
** The following is the implementation of an SQL function that always
** fails with an error message stating that the function is used in the
** wrong context.  The sqlite3_overload_function() API might construct
** SQL function that use this routine so that the functions will exist
** for name resolution but are actually overloaded by the xFindFunction
** method of virtual tables.
*/
 void sqlite3InvalidFunction(
  sqlite3_context *context,  /* The function calling context */
  int NotUsed,               /* Number of arguments to the function */
  sqlite3_value **NotUsed2   /* Value of each argument */
){
  const char *Name = context.pFunc.Name;
  char *zErr;

  zErr = fmt.Sprintf("unable to use function %v in the requested context", Name);
  sqlite3_result_error(context, zErr, -1);
  zErr = nil
}

// Allocate or return the aggregate context for a user function. A new context is allocated on the first call. Subsequent calls return the same context that was returned on prior calls.
func void *sqlite3_aggregate_context(sqlite3_context *p, int nByte){
  Mem *pMem;
  assert( p && p.pFunc && p.pFunc.xStep );
  pMem = p.pMem;
  if( (pMem.flags & MEM_Agg)==0 ){
    if( nByte<=0 ){
      pMem.ReleaseExternal()
      pMem.flags = MEM_Null;
      pMem.z = 0;
    }else{
      sqlite3VdbeMemGrow(pMem, nByte, 0);
      pMem.flags = MEM_Agg;
      pMem.u.pDef = p.pFunc;
      if( pMem.z ){
        memset(pMem.z, 0, nByte);
      }
    }
  }
  return (void*)pMem.z;
}

/*
** Return the auxilary data pointer, if any, for the iArg'th argument to
** the user-function defined by pCtx.
*/
func void *sqlite3_get_auxdata(sqlite3_context *pCtx, int iArg){
  VdbeFunc *pVdbeFunc;

  pVdbeFunc = pCtx.pVdbeFunc;
  if( !pVdbeFunc || iArg>=pVdbeFunc.nAux || iArg<0 ){
    return 0;
  }
  return pVdbeFunc.apAux[iArg].Parameter;
}

/*
** Set the auxilary data pointer and delete function, for the iArg'th
** argument to the user-function defined by pCtx. Any previous value is
** deleted by calling the delete function specified when it was set.
*/
func void sqlite3_set_auxdata(
  sqlite3_context *pCtx, 
  int iArg, 
  void *pAux, 
  void (*xDelete)(void*)
){
  struct AuxData *pAuxData;
  VdbeFunc *pVdbeFunc;
  if( iArg<0 ) goto failed;

  pVdbeFunc = pCtx.pVdbeFunc;
  if( !pVdbeFunc || pVdbeFunc.nAux<=iArg ){
    int nAux = (pVdbeFunc ? pVdbeFunc.nAux : 0);
    int nMalloc = sizeof(VdbeFunc) + sizeof(struct AuxData)*iArg;
    pVdbeFunc = sqlite3DbRealloc(pCtx.s.db, pVdbeFunc, nMalloc);
    if( !pVdbeFunc ){
      goto failed;
    }
    pCtx.pVdbeFunc = pVdbeFunc;
    memset(&pVdbeFunc.apAux[nAux], 0, sizeof(struct AuxData)*(iArg+1-nAux));
    pVdbeFunc.nAux = iArg+1;
    pVdbeFunc.pFunc = pCtx.pFunc;
  }

  pAuxData = &pVdbeFunc.apAux[iArg];
  if( pAuxData.Parameter && pAuxData.xDelete ){
    pAuxData.xDelete(pAuxData.Parameter);
  }
  pAuxData.Parameter = pAux;
  pAuxData.xDelete = xDelete;
  return;

failed:
  if( xDelete ){
    xDelete(pAux);
  }
}

/*
** Return the number of columns in the result set for the statement pStmt.
*/
func int sqlite3_column_count(sqlite3_stmt *pStmt){
  Vdbe *pVm = (Vdbe *)pStmt;
  return pVm ? pVm.nResColumn : 0;
}

/*
** Return the number of values available from the current row of the
** currently executing statement pStmt.
*/
func int sqlite3_data_count(sqlite3_stmt *pStmt){
  Vdbe *pVm = (Vdbe *)pStmt;
  if( pVm==0 || pVm.pResultSet==0 ) return 0;
  return pVm.nResColumn;
}


//	Check to see if column iCol of the given statement is valid. If it is, return a pointer to the Mem for the value of that column. If iCol is not valid, return a pointer to a Mem which has a value of NULL.
func (pStmt *sqlite3_stmt) ColumnMem(i int) (pOut *Mem) {
	pVm := (Vdbe *)(pStmt)
	if pVm != nil && pVm.pResultSet != nil && i < pVm.nResColumn && i >= 0 {
		pVm.db.mutex.Lock()
		pOut = &pVm.pResultSet[i]
	} else {
		//	If the value passed as the second argument is out of range, return a pointer to the following static Mem object which contains the value SQL NULL. Even though the Mem structure contains an element of type int64, on certain architectures (x86) with certain compiler switches (-Os), gcc may align this Mem object on a 4-byte boundary instead of an 8-byte one.
		if pVm && pVm.db {
			pVm.db.mutex.Lock()
			pVm.db.Error(SQLITE_RANGE, "")
		}
		pOut = new(Mem)				//	 0, "", (double)0, {0}, 0, MEM_Null, SQLITE_NULL, 0, 0, 0 };
	}
	return
}

/*
** This function is called after invoking an sqlite3_value_XXX function on a 
** column value (i.e. a value returned by evaluating an SQL expression in the
** select list of a SELECT statement) that may cause a malloc() failure. If 
** malloc() has failed, the threads mallocFailed flag is cleared and the result
** code of statement pStmt set to SQLITE_NOMEM.
**
** Specifically, this is called from within:
**
**     sqlite3_column_int()
**     sqlite3_column_int64()
**     sqlite3_column_text()
**     sqlite3_column_text16()
**     sqlite3_column_real()
**     sqlite3_column_bytes()
**     sqlite3_column_bytes16()
**     sqiite3_column_blob()
*/
static void columnMallocFailure(sqlite3_stmt *pStmt)
{
  /* If malloc() failed during an encoding conversion within an
  ** sqlite3_column_XXX API, then set the return code of the statement to
  ** SQLITE_NOMEM. The next call to _step() (if any) will return SQLITE_ERROR
  ** and _finalize() will return NOMEM.
  */
  Vdbe *p = (Vdbe *)pStmt;
  if( p ){
    p.rc = p.db.ApiExit(p.rc)
    p.db.mutex.Unlock()
  }
}

/**************************** sqlite3_column_  *******************************
** The following routines are used to access elements of the current row
** in the result set.
*/
func const void *sqlite3_column_blob(sqlite3_stmt *pStmt, int i){
  const void *val;
  val = sqlite3_value_blob( pStmt.ColumnMem(i) );
  /* Even though there is no encoding conversion, value_blob() might
  ** need to call malloc() to expand the result of a zeroblob() 
  ** expression. 
  */
  columnMallocFailure(pStmt);
  return val;
}
func int sqlite3_column_bytes(sqlite3_stmt *pStmt, int i){
  int val = sqlite3_value_bytes( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func int sqlite3_column_bytes16(sqlite3_stmt *pStmt, int i){
  int val = sqlite3_value_bytes16( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func double sqlite3_column_double(sqlite3_stmt *pStmt, int i){
  double val = sqlite3_value_double( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func int sqlite3_column_int(sqlite3_stmt *pStmt, int i){
  int val = sqlite3_value_int( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func sqlite_int64 sqlite3_column_int64(sqlite3_stmt *pStmt, int i){
  sqlite_int64 val = sqlite3_value_int64( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func const unsigned char *sqlite3_column_text(sqlite3_stmt *pStmt, int i){
  const unsigned char *val = sqlite3_value_text( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
func sqlite3_value *sqlite3_column_value(sqlite3_stmt *pStmt, int i){
  Mem *pOut = pStmt.ColumnMem(i);
  if( pOut.flags&MEM_Static ){
    pOut.flags &= ~MEM_Static;
    pOut.flags |= MEM_Ephem;
  }
  columnMallocFailure(pStmt);
  return (sqlite3_value *)pOut;
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_text16(sqlite3_stmt *pStmt, int i){
  const void *val = sqlite3_value_text16( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return val;
}
#endif /* SQLITE_OMIT_UTF16 */
func int sqlite3_column_type(sqlite3_stmt *pStmt, int i){
  int iType = sqlite3_value_type( pStmt.ColumnMem(i) );
  columnMallocFailure(pStmt);
  return iType;
}

/* The following function is experimental and subject to change or
** removal */
/*int sqlite3_column_numeric_type(sqlite3_stmt *pStmt, int i){
**  return sqlite3_value_numeric_type( pStmt.ColumnMem(i) );
**}
*/

/*
** Convert the N-th element of pStmt.pColName[] into a string using
** xFunc() then return that string.  If N is out of range, return 0.
**
** There are up to 5 names for each column.  useType determines which
** name is returned.  Here are the names:
**
**    0      The column name as it should be displayed for output
**    1      The datatype name for the column
**    2      The name of the database that the column derives from
**    3      The name of the table that the column derives from
**    4      The name of the table column that the result column derives from
**
** If the result is not a simple column reference (if it is an expression
** or a constant) then useTypes 2, 3, and 4 return NULL.
*/
static const void *columnName(
  sqlite3_stmt *pStmt,
  int N,
  const void *(*xFunc)(Mem*),
  int useType
){
  const void *ret = 0;
  Vdbe *p = (Vdbe *)pStmt;
  int n;
  sqlite3 *db = p.db;
  
  assert( db!=0 );
  n = sqlite3_column_count(pStmt);
  if( N<n && N>=0 ){
    N += useType*n;
    db.mutex.Lock()
    assert( !db.mallocFailed );
    ret = xFunc(&p.ColumnsName[N]);
     /* A malloc may have failed inside of the xFunc() call. If this
    ** is the case, clear the mallocFailed flag and return NULL.
    */
    if( db.mallocFailed ){
      db.mallocFailed = false
      ret = 0;
    }
    db.mutex.Unlock()
  }
  return ret;
}

/*
** Return the name of the Nth column of the result set returned by SQL
** statement pStmt.
*/
func const char *sqlite3_column_name(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text, COLNAME_NAME);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_name16(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text16, COLNAME_NAME);
}
#endif

/*
** Constraint:  If you have ENABLE_COLUMN_METADATA then you must
** not define OMIT_DECLTYPE.
*/
#if defined(SQLITE_OMIT_DECLTYPE) && defined(SQLITE_ENABLE_COLUMN_METADATA)
# error "Must not define both SQLITE_OMIT_DECLTYPE \
         and SQLITE_ENABLE_COLUMN_METADATA"
#endif

#ifndef SQLITE_OMIT_DECLTYPE
/*
** Return the column declaration type (if applicable) of the 'i'th column
** of the result set of SQL statement pStmt.
*/
func const char *sqlite3_column_decltype(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text, COLNAME_DECLTYPE);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_decltype16(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text16, COLNAME_DECLTYPE);
}
#endif /* SQLITE_OMIT_UTF16 */
#endif /* SQLITE_OMIT_DECLTYPE */

#ifdef SQLITE_ENABLE_COLUMN_METADATA
/*
** Return the name of the database from which a result column derives.
** NULL is returned if the result column is an expression or constant or
** anything else which is not an unabiguous reference to a database column.
*/
func const char *sqlite3_column_database_name(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text, COLNAME_DATABASE);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_database_name16(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text16, COLNAME_DATABASE);
}
#endif /* SQLITE_OMIT_UTF16 */

/*
** Return the name of the table from which a result column derives.
** NULL is returned if the result column is an expression or constant or
** anything else which is not an unabiguous reference to a database column.
*/
func const char *sqlite3_column_table_name(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text, COLNAME_TABLE);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_table_name16(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text16, COLNAME_TABLE);
}
#endif /* SQLITE_OMIT_UTF16 */

/*
** Return the name of the table column from which a result column derives.
** NULL is returned if the result column is an expression or constant or
** anything else which is not an unabiguous reference to a database column.
*/
func const char *sqlite3_column_origin_name(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text, COLNAME_COLUMN);
}
#ifndef SQLITE_OMIT_UTF16
func const void *sqlite3_column_origin_name16(sqlite3_stmt *pStmt, int N){
  return columnName(
      pStmt, N, (const void*(*)(Mem*))sqlite3_value_text16, COLNAME_COLUMN);
}
#endif /* SQLITE_OMIT_UTF16 */
#endif /* SQLITE_ENABLE_COLUMN_METADATA */


//	Unbind the value bound to variable i in virtual machine p. This is the the same as binding a NULL value to the column. If the "i" parameter is out of range, then SQLITE_RANGE is returned. Othewise SQLITE_OK.
//	A successful evaluation of this routine acquires the mutex on p. The mutex is released if any kind of error occurs.
//	The error code stored in database p.db is overwritten with the return value in any case.
func (p *Vdbe) Unbind(i int) (rc int) {
	if vdbeSafetyNotNull(p) {
		return SQLITE_MISUSE_BKPT
	}
	p.db.mutex.Lock()
	if p.magic != VDBE_MAGIC_RUN || p.pc >= 0 {
		p.db.Error(SQLITE_MISUSE, "")
		p.db.mutex.Unlock()
		sqlite3_log(SQLITE_MISUSE, "bind on a busy prepared statement: [%s]", p.zSql)
		return SQLITE_MISUSE_BKPT
	}
	if i < 1 || i > p.nVar {
		p.db.Error(SQLITE_RANGE, "")
		p.db.mutex.Unlock()
		return SQLITE_RANGE
	}
	i--
	pVar := p.aVar[i]
	pVar.Release()
	p.db.Error(SQLITE_OK, "")

	//	If the bit corresponding to this variable in Vdbe.expmask is set, then binding a new value to this variable invalidates the current query plan.
	//	IMPLEMENTATION-OF: R-48440-37595 If the specific value bound to host parameter in the WHERE clause might influence the choice of query plan for a statement, then the statement will be automatically recompiled, as if there had been a schema change, on the first sqlite3_step() call following any change to the bindings of that parameter.
	if p.isPrepareV2 && (i < 32 && p.expmask & (uint32(1) << i) || p.expmask == 0xffffffff {
		p.expired = 1
	}
	return
}

//	Bind a text or BLOB value.
func (pStmt *sqlite3_stmt) BindText(i int, zData string, xDel func(interface{}) interface{}, encoding byte) (rc int) {
static int bindText(
  sqlite3_stmt *pStmt,   /* The statement to bind against */
  int i,                 /* Index of the parameter to bind */
  const void *zData,     /* Pointer to the data to be bound */
  int nData,             /* Number of bytes of data to be bound */
  void (*xDel)(void*),   /* Destructor for the data */
  byte encoding            /* Encoding for the data */
){
	p := (Vdbe *)(pStmt)
	if rc = p.Unbind(i); rc == SQLITE_OK {
		if zData != "" {
			pVar := &p.aVar[i-1]
			rc = pVar.SetStr(zData, encoding, xDel)
			if rc == SQLITE_OK && encoding != 0 {
				rc = sqlite3VdbeChangeEncoding(pVar, p.db.Encoding())
			}
			p.db.Error(rc, "")
			rc = p.db.ApiExit(rc)
		}
		p.db.mutex.Unlock()
	} else if xDel != SQLITE_STATIC && xDel != SQLITE_TRANSIENT {
		xDel((void*)zData)
	}
	return
}


//	Bind a blob value to an SQL statement variable.
func (pStmt *sqlite3_stmt) BindBlob(i int, data string, xDel func(interface{}) interface{}) int {
	 return pStmt.bindText(i, data, xDel, 0)
}

func int sqlite3_bind_double(sqlite3_stmt *pStmt, int i, double rValue){
  int rc;
  p := (Vdbe *)(pStmt)
  if rc = p.Unbind(i); rc == SQLITE_OK {
    sqlite3VdbeMemSetDouble(&p.aVar[i-1], rValue)
    p.db.mutex.Unlock()
  }
  return rc
}
func int sqlite3_bind_int(sqlite3_stmt *p, int i, int Value){
  return sqlite3_bind_int64(p, i, (int64)Value);
}
func int sqlite3_bind_int64(sqlite3_stmt *pStmt, int i, sqlite_int64 Value){
  int rc
  p := (Vdbe *)(pStmt)
  if rc = p.Unbind(i); rc == SQLITE_OK {
    p.aVar[i-1].SetInt64(Value)
    p.db.mutex.Unlock()
  }
  return rc
}
func int sqlite3_bind_null(sqlite3_stmt *pStmt, int i){
  int rc
  p := (Vdbe*)(pStmt)
  if rc = p.Unbind(i); rc == SQLITE_OK {
    p.db.mutex.Unlock()
  }
  return rc
}
func int sqlite3_bind_text( 
  sqlite3_stmt *pStmt, 
  int i, 
  const char *zData, 
  int nData, 
  void (*xDel)(void*)
){
  return bindText(pStmt, i, zData, nData, xDel, SQLITE_UTF8);
}
#ifndef SQLITE_OMIT_UTF16
func int sqlite3_bind_text16(
  sqlite3_stmt *pStmt, 
  int i, 
  const void *zData, 
  int nData, 
  void (*xDel)(void*)
){
  return bindText(pStmt, i, zData, nData, xDel, SQLITE_UTF16NATIVE);
}
#endif /* SQLITE_OMIT_UTF16 */

func (pStmt *sqlite3_stmt) BindValue(i int, pValue *Mem) (rc int) {
	switch pValue.Type {
	case SQLITE_INTEGER:
		rc = sqlite3_bind_int64(pStmt, i, pValue.Value)
	case SQLITE_FLOAT:
		rc = sqlite3_bind_double(pStmt, i, pValue.r)
	case SQLITE_BLOB:
		if v, ok := pValue.Value.(Zeroes); ok {
			rc = pStmt.BindZeroBlob(i, v)
		} else {
			rc = pStmt.BindBlob(i, pValue.z, SQLITE_TRANSIENT)
		}
    case SQLITE_TEXT:
		rc = bindText(pStmt,i,  pValue.z, pValue.n, SQLITE_TRANSIENT, pValue.enc)
	default:
		rc = sqlite3_bind_null(pStmt, i)
	}
	return
}

func (pStmt *sqlite3_stmt) BindZeroBlob(i int, n Zeroes) (rc int) {
	p := (Vdbe *)(pStmt)
	if rc = p.Unbind(i); rc == SQLITE_OK {
		p.aVar[i-1].SetZeroBlob(n)
		p.db.mutex.Unlock()
	}
	return
}

/*
** Return the number of wildcards that can be potentially bound to.
** This routine is added to support DBD::SQLite.  
*/
func int sqlite3_bind_parameter_count(sqlite3_stmt *pStmt){
  Vdbe *p = (Vdbe*)pStmt;
  return p ? p.nVar : 0;
}

/*
** Return the name of a wildcard parameter.  Return NULL if the index
** is out of range or if the wildcard is unnamed.
**
** The result is always UTF-8.
*/
func const char *sqlite3_bind_parameter_name(sqlite3_stmt *pStmt, int i){
  Vdbe *p = (Vdbe*)pStmt;
  if( p==0 || i<1 || i>p.nzVar ){
    return 0;
  }
  return p.azVar[i-1];
}

/*
** Given a wildcard parameter name, return the index of the variable
** with that name.  If there is no variable with the given name,
** return 0.
*/
 int sqlite3VdbeParameterIndex(Vdbe *p, const char *Name, int nName){
  int i;
  if( p==0 ){
    return 0;
  }
  if( Name ){
    for(i=0; i<p.nzVar; i++){
      const char *z = p.azVar[i];
      if( z && memcmp(z,Name,nName)==0 && z[nName]==0 ){
        return i+1;
      }
    }
  }
  return 0;
}
func int sqlite3_bind_parameter_index(sqlite3_stmt *pStmt, const char *Name){
  return sqlite3VdbeParameterIndex((Vdbe*)pStmt, Name, sqlite3Strlen30(Name));
}

/*
** Transfer all bindings from the first statement over to the second.
*/
 int sqlite3TransferBindings(sqlite3_stmt *pFromStmt, sqlite3_stmt *pToStmt){
  Vdbe *pFrom = (Vdbe*)pFromStmt;
  Vdbe *pTo = (Vdbe*)pToStmt;
  int i;
  assert( pTo.db==pFrom.db );
  assert( pTo.nVar==pFrom.nVar );
  pTo.db.mutex.Lock()
  for(i=0; i<pFrom.nVar; i++){
    sqlite3VdbeMemMove(&pTo.aVar[i], &pFrom.aVar[i]);
  }
  pTo.db.mutex.Unlock()
  return SQLITE_OK;
}

/*
** Return the sqlite3* database handle to which the prepared statement given
** in the argument belongs.  This is the same database handle that was
** the first argument to the sqlite3_prepare() that was used to create
** the statement in the first place.
*/
func sqlite3 *sqlite3_db_handle(sqlite3_stmt *pStmt){
  return pStmt ? ((Vdbe*)pStmt).db : 0;
}

/*
** Return true if the prepared statement is guaranteed to not modify the
** database.
*/
func int sqlite3_stmt_readonly(sqlite3_stmt *pStmt){
  return pStmt ? ((Vdbe*)pStmt).readOnly : 1;
}

/*
** Return true if the prepared statement is in need of being reset.
*/
func int sqlite3_stmt_busy(sqlite3_stmt *pStmt){
  Vdbe *v = (Vdbe*)pStmt;
  return v!=0 && v.pc>0 && v.magic==VDBE_MAGIC_RUN;
}

/*
** Return a pointer to the next prepared statement after pStmt associated
** with database connection pDb.  If pStmt is NULL, return the first
** prepared statement for the database connection.  Return NULL if there
** are no more.
*/
func sqlite3_stmt *sqlite3_next_stmt(sqlite3 *pDb, sqlite3_stmt *pStmt){
  sqlite3_stmt *Next;
  pDb.mutex.Lock()
  if( pStmt==0 ){
    Next = (sqlite3_stmt*)pDb.pVdbe;
  }else{
    Next = (sqlite3_stmt*)((Vdbe*)pStmt).Next;
  }
  pDb.mutex.Unlock()
  return Next;
}

/*
** Return the value of a status counter for a prepared statement
*/
func int sqlite3_stmt_status(sqlite3_stmt *pStmt, int op, int resetFlag){
  Vdbe *pVdbe = (Vdbe*)pStmt;
  int v = pVdbe.aCounter[op-1];
  if( resetFlag ) pVdbe.aCounter[op-1] = 0;
  return v;
}
