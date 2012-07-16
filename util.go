/* Utility functions used throughout sqlite.
**
** This file contains functions for allocating memory, comparing
** strings, and stuff like that.
**
*/
/* #include <stdarg.h> */

/*
** Compute a string length that is limited to what can be stored in
** lower 30 bits of a 32-bit signed integer.
**
** The value returned will never be negative.  Nor will it ever be greater
** than the actual length of the string.  For very long strings (greater
** than 1GiB) the value returned might be less than the true string length.
*/
 int sqlite3Strlen30(const char *z){
  const char *z2 = z;
  if( z==0 ) return 0;
  while( *z2 ){ z2++; }
  return 0x3fffffff & (int)(z2 - z);
}

/*
** Set the most recent error code and error string for the sqlite
** handle "db". The error code is set to "err_code".
**
** If it is not NULL, string zFormat specifies the format of the
** error string in the style of the printf functions: The following
** format characters are allowed:
**
**      %s      Insert a string
**      %z      A string that should be freed after use
**      %d      Insert an integer
**      %T      Insert a token
**      %S      Insert the first element of a SrcList
**
** zFormat and any string tokens that follow it are assumed to be
** encoded in UTF-8.
**
** To clear the most recent error for sqlite handle "db", Error
** should be called with err_code set to SQLITE_OK and zFormat set
** to NULL.
*/
func (db *sqlite3) Error(err_code int, format string, ap... string) {
	if db != nil && (db.pErr || (db.pErr = db.NewValue() != 0) ) {
		db.errCode = err_code
		if format != "" {
			z := fmt.Sprintf("", zFormat, ap)
			sqlite3ValueSetStr(db.pErr, -1, z, SQLITE_UTF8, SQLITE_DYNAMIC)
		} else {
			sqlite3ValueSetStr(db.pErr, 0, 0, SQLITE_UTF8, SQLITE_STATIC)
		}
	}
}

//	Add an error message to pParse.zErrMsg and increment pParse.nErr.
//	This function should be used to report any error that occurs whilst compiling an SQL statement (i.e. within sqlite3_prepare()). The last thing the sqlite3_prepare() function does is copy the error stored by this function into the database handle using Error(). Function Error() should be used during statement execution (sqlite3_step() etc.).
func (pParse *Parse) SetErrorMsg(format string, terms ...string) {
	if !db.suppressErr {
		db := pParse.db
		zMsg := fmt.Sprintf(format, terms...)
		pParse.nErr++
		pParse.zErrMsg = zMsg
		pParse.rc = SQLITE_ERROR
	}
}

//	Convert an SQL-style quoted string into a normal string by removing the quote characters. The conversion is done in-place. If the input does not begin with a quote character, then this routine is a no-op.
//	The return value is -1 if no dequoting occurs or the length of the dequoted string, if dequoting does occur.
//	2002-Feb-14: This routine is extended to remove MS-Access style brackets from around identifers.  For example:  "[a-b-c]" becomes "a-b-c".
func Dequote(z string) (r string) {
	if len(z) != 0 {
	  	quote := z[0]
	  	switch quote {
	  	case '\'':
	  	case '"':
	  	case '`':					//	For MySQL compatibility
	  	case '[':   quote = ']'		//	For MS SqlServer compatibility
	  	default:    return z
	  	}
		for i := 1; i < len(z); i++ {
			if z[i] == quote {
				if z[i + 1] == quote {
					r = append(r, quote)
					i++
				} else {
					break
				}
			} else {
				r = append(r, z[i])
			}
		}
	}
	return
}

/* Convenient short-hand */
#define UpperToLower sqlite3UpperToLower

func CaseInsensitiveMatch(left, right string) (ok bool) {
	if len(left) == len(right) {
		return strings.ToLower(left) == strings.ToLower(right)
	}
}

func CaseInsensitiveMatchN(left, right string, n int) (ok bool) {
	return CaseInsensitiveMatch(left[:n], right[:n])
}

//	Compare the 19-character string zNum against the text representation value 2^63: 9223372036854775808. Return negative, zero, or positive if zNum is less than, equal to, or greater than the string. Note that zNum must contain exactly 19 characters.
//	Unlike memcmp() this routine is guaranteed to return the difference in the values of the last digit if the only difference is in the last digit. So, for example,
//			compare2pow63("9223372036854775800", 1)
//	will return -8.
static int compare2pow63(const char *zNum, int incr){
  int c = 0;
  int i;
                    /* 012345678901234567 */
  const char *pow63 = "922337203685477580";
  for(i=0; c==0 && i<18; i++){
    c = (zNum[i*incr]-pow63[i])*10;
  }
  if( c==0 ){
    c = zNum[18*incr] - '8';
  }
  return c;
}

/*
** If zNum represents an integer that will fit in 32-bits, then set
** *pValue to that integer and return true.  Otherwise return false.
**
** Any non-numeric characters that following zNum are ignored.
*/
 int sqlite3GetInt32(const char *zNum, int *pValue){
  sqlite_int64 v = 0;
  int i, c;
  int neg = 0;
  if( zNum[0]=='-' ){
    neg = 1;
    zNum++;
  }else if( zNum[0]=='+' ){
    zNum++;
  }
  while( zNum[0]=='0' ) zNum++;
  for(i=0; i<11 && (c = zNum[i] - '0')>=0 && c<=9; i++){
    v = v*10 + c;
  }

  /* The longest decimal representation of a 32 bit integer is 10 digits:
  **
  **             1234567890
  **     2^31 . 2147483648
  */
  if( i>10 ){
    return 0;
  }
  if( v-neg>2147483647 ){
    return 0;
  }
  if( neg ){
    v = -v;
  }
  *pValue = (int)v;
  return 1;
}

/*
** Return a 32-bit integer value extracted from a string.  If the
** string is not an integer, just return 0.
*/
 int sqlite3Atoi(const char *z){
  int x = 0;
  if( z ) sqlite3GetInt32(z, &x);
  return x;
}

/*
** Translate a single byte of Hex into an integer.
** This routine only works if h really is a valid hexadecimal
** character:  0..9a..fA..F
*/
 byte sqlite3HexToInt(int h){
  assert( (h>='0' && h<='9') ||  (h>='a' && h<='f') ||  (h>='A' && h<='F') );
  h += 9*(1&(h>>6));
  return (byte)(h & 0xf);
}

/*
** Convert a BLOB literal of the form "x'hhhhhh'" into its binary
** value.  Return a pointer to its binary value.  Space to hold the
** binary value has been obtained from malloc and must be freed by
** the calling routine.
*/
 void *sqlite3HexToBlob(sqlite3 *db, const char *z, int n){
  char *zBlob;
  int i;

  zBlob = (char *)sqlite3DbMallocRaw(db, n/2 + 1);
  n--;
  if( zBlob ){
    for(i=0; i<n; i+=2){
      zBlob[i/2] = (sqlite3HexToInt(z[i])<<4) | sqlite3HexToInt(z[i+1]);
    }
    zBlob[i/2] = 0;
  }
  return zBlob;
}

//	Log an error that is an API call on a connection pointer that should not have been used.
func logBadConnection(message string){
	sqlite3_log(SQLITE_MISUSE, "API call with %v database connection pointer", message)
}

//	Check to make sure we have a valid db pointer. This test is not foolproof but it does provide some measure of protection against misuse of the interface such as passing in db pointers that are nil or which have been previously closed. If this routine returns true it means that the db pointer is valid and false if it should not be dereferenced for any reason. The calling function should invoke SQLITE_MISUSE immediately.
//	SafetyCheckOk() requires that the db pointer be valid for use. SafetyCheckSickOrOk() allows a db pointer that failed to open properly and is not fit for general use but which can be used as an argument to sqlite3_errmsg() or sqlite3::Close().
func (db *sqlite3) SafetyCheckOk() bool {
	if db == 0 {
		logBadConnection("NULL")
		return false
	}
	if magic := db.magic; magic != SQLITE_MAGIC_OPEN {
		if db.SafetyCheckSickOrOk() {
			logBadConnection("unopened")
		}
		return false
	}
	return true
}

func (db *sqlite3) SafetyCheckSickOrOk() bool {
	if magic := db.magic; magic != SQLITE_MAGIC_SICK && magic != SQLITE_MAGIC_OPEN && magic != SQLITE_MAGIC_BUSY {
		logBadConnection("invalid")
		return false
	}
	return true
}

/*
** Attempt to add, substract, or multiply the 64-bit signed value iB against
** the other 64-bit signed integer at *pA and store the result in *pA.
** Return 0 on success.  Or if the operation would have resulted in an
** overflow, leave *pA unchanged and return 1.
*/
 int sqlite3AddInt64(int64 *pA, int64 iB){
  int64 iA = *pA;
  if( iB>=0 ){
    if( iA>0 && LARGEST_INT64 - iA < iB ) return 1;
    *pA += iB;
  }else{
    if( iA<0 && -(iA + LARGEST_INT64) > iB + 1 ) return 1;
    *pA += iB;
  }
  return 0; 
}
 int sqlite3SubInt64(int64 *pA, int64 iB){
  if( iB==SMALLEST_INT64 ){
    if( (*pA)>=0 ) return 1;
    *pA -= iB;
    return 0;
  }else{
    return sqlite3AddInt64(pA, -iB);
  }
}
#define TWOPOWER32 (((int64)1)<<32)
#define TWOPOWER31 (((int64)1)<<31)
 int sqlite3MulInt64(int64 *pA, int64 iB){
  int64 iA = *pA;
  int64 iA1, iA0, iB1, iB0, r;

  iA1 = iA/TWOPOWER32;
  iA0 = iA % TWOPOWER32;
  iB1 = iB/TWOPOWER32;
  iB0 = iB % TWOPOWER32;
  if( iA1*iB1 != 0 ) return 1;
  assert( iA1*iB0==0 || iA0*iB1==0 );
  r = iA1*iB0 + iA0*iB1;
  if( r<(-TWOPOWER31) || r>=TWOPOWER31 ) return 1;
  r *= TWOPOWER32;
  if( sqlite3AddInt64(&r, iA0*iB0) ) return 1;
  *pA = r;
  return 0;
}

/*
** Compute the absolute value of a 32-bit signed integer, of possible.  Or 
** if the integer has a value of -2147483648, return +2147483647
*/
 int sqlite3AbsInt32(int x){
  if( x>=0 ) return x;
  if( x==(int)0x80000000 ) return 0x7fffffff;
  return -x;
}