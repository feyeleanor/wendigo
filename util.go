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
** The variable-length integer encoding is as follows:
**
** KEY:
**         A = 0xxxxxxx    7 bits of data and one flag bit
**         B = 1xxxxxxx    7 bits of data and one flag bit
**         C = xxxxxxxx    8 bits of data
**
**  7 bits - A
** 14 bits - BA
** 21 bits - BBA
** 28 bits - BBBA
** 35 bits - BBBBA
** 42 bits - BBBBBA
** 49 bits - BBBBBBA
** 56 bits - BBBBBBBA
** 64 bits - BBBBBBBBC
*/

/*
** Write a 64-bit variable-length integer to memory starting at p[0].
** The length of data write will be between 1 and 9 bytes.  The number
** of bytes written is returned.
**
** A variable-length integer consists of the lower 7 bits of each byte
** for all bytes that have the 8th bit set and one byte with the 8th
** bit clear.  Except, if we get to the 9th byte, it stores the full
** 8 bits and is the last byte.
*/
 int sqlite3PutVarint(unsigned char *p, uint64 v){
  int i, j, n;
  byte buf[10];
  if( v & (((uint64)0xff000000)<<32) ){
    p[8] = (byte)v;
    v >>= 8;
    for(i=7; i>=0; i--){
      p[i] = (byte)((v & 0x7f) | 0x80);
      v >>= 7;
    }
    return 9;
  }    
  n = 0;
  do{
    buf[n++] = (byte)((v & 0x7f) | 0x80);
    v >>= 7;
  }while( v!=0 );
  buf[0] &= 0x7f;
  assert( n<=9 );
  for(i=0, j=n-1; j>=0; j--, i++){
    p[i] = buf[j];
  }
  return n;
}

/*
** This routine is a faster version of sqlite3PutVarint() that only
** works for 32-bit positive integers and which is optimized for
** the common case of small integers.  A MACRO version, putVarint32,
** is provided which inlines the single-byte case.  All code should use
** the MACRO version as this function assumes the single-byte case has
** already been handled.
*/
 int sqlite3PutVarint32(unsigned char *p, uint32 v){
#ifndef putVarint32
  if( (v & ~0x7f)==0 ){
    p[0] = v;
    return 1;
  }
#endif
  if( (v & ~0x3fff)==0 ){
    p[0] = (byte)((v>>7) | 0x80);
    p[1] = (byte)(v & 0x7f);
    return 2;
  }
  return sqlite3PutVarint(p, v);
}

//	Bitmasks used by sqlite3GetVarint().
//		SLOT_2_0     A mask for  (0x7f<<14) | 0x7f
//		SLOT_4_2_0   A mask for  (0x7f<<28) | SLOT_2_0
const (
	SLOT_2_0 = 0x001fc07f
	SLOT_4_2_0 = 0xf01fc07f
)

//	Read a 64-bit variable-length integer from memory starting at p[0]. Return the number of bytes read. The value is stored in *v.
func GetVarint(p []byte) (v uint64, count int) {
byte sqlite3GetVarint(const unsigned char *p, uint64 *v){
	a, b, s		uint32

	a = uint32(p[0])		//	a: p0 (unmasked)
	if a & 0x80 == 0 {
		v = a
		return 1
	}

	b = uint32(b[1])		//	b: p1 (unmasked)
	if b & 0x80 == 0 {
		a &= 0x7f
		a = a << 7
		a |= b
		v = a
		return 2
	}

	//	Verify that constants are precomputed correctly
	assert( SLOT_2_0 == ((0x7f << 14) | 0x7f) )
	assert( SLOT_4_2_0 == ((0xfU << 28) | (0x7f << 14) | 0x7f) )

  p++;
  a = a<<14;
  a |= *p;
  /* a: p0<<14 | p2 (unmasked) */
  if (!(a&0x80))
  {
    a &= SLOT_2_0;
    b &= 0x7f;
    b = b<<7;
    a |= b;
    *v = a;
    return 3;
  }

  /* CSE1 from below */
  a &= SLOT_2_0;
  p++;
  b = b<<14;
  b |= *p;
  /* b: p1<<14 | p3 (unmasked) */
  if (!(b&0x80))
  {
    b &= SLOT_2_0;
    /* moved CSE1 up */
    /* a &= (0x7f<<14)|(0x7f); */
    a = a<<7;
    a |= b;
    *v = a;
    return 4;
  }

  /* a: p0<<14 | p2 (masked) */
  /* b: p1<<14 | p3 (unmasked) */
  /* 1:save off p0<<21 | p1<<14 | p2<<7 | p3 (masked) */
  /* moved CSE1 up */
  /* a &= (0x7f<<14)|(0x7f); */
  b &= SLOT_2_0;
  s = a;
  /* s: p0<<14 | p2 (masked) */

  p++;
  a = a<<14;
  a |= *p;
  /* a: p0<<28 | p2<<14 | p4 (unmasked) */
  if (!(a&0x80))
  {
    /* we can skip these cause they were (effectively) done above in calc'ing s */
    /* a &= (0x7f<<28)|(0x7f<<14)|(0x7f); */
    /* b &= (0x7f<<14)|(0x7f); */
    b = b<<7;
    a |= b;
    s = s>>18;
    *v = ((uint64)s)<<32 | a;
    return 5;
  }

  /* 2:save off p0<<21 | p1<<14 | p2<<7 | p3 (masked) */
  s = s<<7;
  s |= b;
  /* s: p0<<21 | p1<<14 | p2<<7 | p3 (masked) */

  p++;
  b = b<<14;
  b |= *p;
  /* b: p1<<28 | p3<<14 | p5 (unmasked) */
  if (!(b&0x80))
  {
    /* we can skip this cause it was (effectively) done above in calc'ing s */
    /* b &= (0x7f<<28)|(0x7f<<14)|(0x7f); */
    a &= SLOT_2_0;
    a = a<<7;
    a |= b;
    s = s>>18;
    *v = ((uint64)s)<<32 | a;
    return 6;
  }

  p++;
  a = a<<14;
  a |= *p;
  /* a: p2<<28 | p4<<14 | p6 (unmasked) */
  if (!(a&0x80))
  {
    a &= SLOT_4_2_0;
    b &= SLOT_2_0;
    b = b<<7;
    a |= b;
    s = s>>11;
    *v = ((uint64)s)<<32 | a;
    return 7;
  }

  /* CSE2 from below */
  a &= SLOT_2_0;
  p++;
  b = b<<14;
  b |= *p;
  /* b: p3<<28 | p5<<14 | p7 (unmasked) */
  if (!(b&0x80))
  {
    b &= SLOT_4_2_0;
    /* moved CSE2 up */
    /* a &= (0x7f<<14)|(0x7f); */
    a = a<<7;
    a |= b;
    s = s>>4;
    *v = ((uint64)s)<<32 | a;
    return 8;
  }

  p++;
  a = a<<15;
  a |= *p;
  /* a: p4<<29 | p6<<15 | p8 (unmasked) */

  /* moved CSE2 up */
  /* a &= (0x7f<<29)|(0x7f<<15)|(0xff); */
  b &= SLOT_2_0;
  b = b<<8;
  a |= b;

  s = s<<4;
  b = p[-4];
  b &= 0x7f;
  b = b>>3;
  s |= b;

  *v = ((uint64)s)<<32 | a;

  return 9;
}


/*
** Return the number of bytes that will be needed to store the given
** 64-bit integer.
*/
 int sqlite3VarintLen(uint64 v){
  int i = 0;
  do{
    i++;
    v >>= 7;
  }while( v!=0 && i<9 );
  return i;
}


//	Read or write a four-byte big-endian integer value.
func Get4Byte(p []byte) uint32
	return (p[0] << 24) | (p[1] << 16) | (p[2] << 8) | p[3]
}

func Put4Byte(p []byte, v uint32) {
	p[0] = byte(v >> 24)
	p[1] = byte(v >> 16)
	p[2] = byte(v >> 8)
	p[3] = byte(v)
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