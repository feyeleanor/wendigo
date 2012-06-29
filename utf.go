/* This file contains routines used to translate between UTF-8, 
** UTF-16, UTF-16BE, and UTF-16LE.
**
** Notes on UTF-8:
**
**   Byte-0    Byte-1    Byte-2    Byte-3    Value
**  0xxxxxxx                                 00000000 00000000 0xxxxxxx
**  110yyyyy  10xxxxxx                       00000000 00000yyy yyxxxxxx
**  1110zzzz  10yyyyyy  10xxxxxx             00000000 zzzzyyyy yyxxxxxx
**  11110uuu  10uuzzzz  10yyyyyy  10xxxxxx   000uuuuu zzzzyyyy yyxxxxxx
**
**
** Notes on UTF-16:  (with wwww+1==uuuuu)
**
**      Word-0               Word-1          Value
**  110110ww wwzzzzyy   110111yy yyxxxxxx    000uuuuu zzzzyyyy yyxxxxxx
**  zzzzyyyy yyxxxxxx                        00000000 zzzzyyyy yyxxxxxx
**
**
** BOM or Byte Order Mark:
**     0xff 0xfe   little-endian utf-16 follows
**     0xfe 0xff   big-endian utf-16 follows
**
*/
/* #include <assert.h> */

/*
** This lookup table is used to help decode the first byte of
** a multi-byte UTF8 character.
*/
static const unsigned char sqlite3Utf8Trans1[] = {
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
  0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
  0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
  0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
  0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x00, 0x00,
};


#define WRITE_UTF8(zOut, c) {                          \
  if( c<0x00080 ){                                     \
    *zOut++ = (byte)(c&0xFF);                            \
  }                                                    \
  else if( c<0x00800 ){                                \
    *zOut++ = 0xC0 + (byte)((c>>6)&0x1F);                \
    *zOut++ = 0x80 + (byte)(c & 0x3F);                   \
  }                                                    \
  else if( c<0x10000 ){                                \
    *zOut++ = 0xE0 + (byte)((c>>12)&0x0F);               \
    *zOut++ = 0x80 + (byte)((c>>6) & 0x3F);              \
    *zOut++ = 0x80 + (byte)(c & 0x3F);                   \
  }else{                                               \
    *zOut++ = 0xF0 + (byte)((c>>18) & 0x07);             \
    *zOut++ = 0x80 + (byte)((c>>12) & 0x3F);             \
    *zOut++ = 0x80 + (byte)((c>>6) & 0x3F);              \
    *zOut++ = 0x80 + (byte)(c & 0x3F);                   \
  }                                                    \
}

#define WRITE_UTF16LE(zOut, c) {                                    \
  if( c<=0xFFFF ){                                                  \
    *zOut++ = (byte)(c&0x00FF);                                       \
    *zOut++ = (byte)((c>>8)&0x00FF);                                  \
  }else{                                                            \
    *zOut++ = (byte)(((c>>10)&0x003F) + (((c-0x10000)>>10)&0x00C0));  \
    *zOut++ = (byte)(0x00D8 + (((c-0x10000)>>18)&0x03));              \
    *zOut++ = (byte)(c&0x00FF);                                       \
    *zOut++ = (byte)(0x00DC + ((c>>8)&0x03));                         \
  }                                                                 \
}

#define WRITE_UTF16BE(zOut, c) {                                    \
  if( c<=0xFFFF ){                                                  \
    *zOut++ = (byte)((c>>8)&0x00FF);                                  \
    *zOut++ = (byte)(c&0x00FF);                                       \
  }else{                                                            \
    *zOut++ = (byte)(0x00D8 + (((c-0x10000)>>18)&0x03));              \
    *zOut++ = (byte)(((c>>10)&0x003F) + (((c-0x10000)>>10)&0x00C0));  \
    *zOut++ = (byte)(0x00DC + ((c>>8)&0x03));                         \
    *zOut++ = (byte)(c&0x00FF);                                       \
  }                                                                 \
}

#define READ_UTF16LE(zIn, TERM, c){                                   \
  c = (*zIn++);                                                       \
  c += ((*zIn++)<<8);                                                 \
  if( c>=0xD800 && c<0xE000 && TERM ){                                \
    int c2 = (*zIn++);                                                \
    c2 += ((*zIn++)<<8);                                              \
    c = (c2&0x03FF) + ((c&0x003F)<<10) + (((c&0x03C0)+0x0040)<<10);   \
  }                                                                   \
}

#define READ_UTF16BE(zIn, TERM, c){                                   \
  c = ((*zIn++)<<8);                                                  \
  c += (*zIn++);                                                      \
  if( c>=0xD800 && c<0xE000 && TERM ){                                \
    int c2 = ((*zIn++)<<8);                                           \
    c2 += (*zIn++);                                                   \
    c = (c2&0x03FF) + ((c&0x003F)<<10) + (((c&0x03C0)+0x0040)<<10);   \
  }                                                                   \
}

/*
** Translate a single UTF-8 character.  Return the unicode value.
**
** During translation, assume that the byte that zTerm points
** is a 0x00.
**
** Write a pointer to the next unread byte back into *pzNext.
**
** Notes On Invalid UTF-8:
**
**  *  This routine never allows a 7-bit character (0x00 through 0x7f) to
**     be encoded as a multi-byte character.  Any multi-byte character that
**     attempts to encode a value between 0x00 and 0x7f is rendered as 0xfffd.
**
**  *  This routine never allows a UTF16 surrogate value to be encoded.
**     If a multi-byte character attempts to encode a value between
**     0xd800 and 0xe000 then it is rendered as 0xfffd.
**
**  *  Bytes in the range of 0x80 through 0xbf which occur as the first
**     byte of a character are interpreted as single-byte characters
**     and rendered as themselves even though they are technically
**     invalid characters.
**
**  *  This routine accepts an infinite number of different UTF8 encodings
**     for unicode values 0x80 and greater.  It do not change over-length
**     encodings to 0xfffd as some systems recommend.
*/
#define READ_UTF8(zIn, zTerm, c)                           \
  c = *(zIn++);                                            \
  if( c>=0xc0 ){                                           \
    c = sqlite3Utf8Trans1[c-0xc0];                         \
    while( zIn!=zTerm && (*zIn & 0xc0)==0x80 ){            \
      c = (c<<6) + (0x3f & *(zIn++));                      \
    }                                                      \
    if( c<0x80                                             \
        || (c&0xFFFFF800)==0xD800                          \
        || (c&0xFFFFFFFE)==0xFFFE ){  c = 0xFFFD; }        \
  }
 uint32 sqlite3Utf8Read(
  const unsigned char *zIn,       /* First byte of UTF-8 character */
  const unsigned char **pzNext    /* Write first byte past UTF-8 char here */
){
  uint c;

  /* Same as READ_UTF8() above but without the zTerm parameter.
  ** For this routine, we assume the UTF8 string is always zero-terminated.
  */
  c = *(zIn++);
  if( c>=0xc0 ){
    c = sqlite3Utf8Trans1[c-0xc0];
    while( (*zIn & 0xc0)==0x80 ){
      c = (c<<6) + (0x3f & *(zIn++));
    }
    if( c<0x80
        || (c&0xFFFFF800)==0xD800
        || (c&0xFFFFFFFE)==0xFFFE ){  c = 0xFFFD; }
  }
  *pzNext = zIn;
  return c;
}




/*
** If the TRANSLATE_TRACE macro is defined, the value of each Mem is
** printed on stderr on the way into and out of sqlite3VdbeMemTranslate().
*/ 
/* #define TRANSLATE_TRACE 1 */

#ifndef SQLITE_OMIT_UTF16
/*
** This routine transforms the internal text encoding used by pMem to
** desiredEnc. It is an error if the string is already of the desired
** encoding, or if *pMem does not contain a string value.
*/
 int sqlite3VdbeMemTranslate(Mem *pMem, byte desiredEnc){
  int len;                    /* Maximum length of output string in bytes */
  unsigned char *zOut;                  /* Output buffer */
  unsigned char *zIn;                   /* Input iterator */
  unsigned char *zTerm;                 /* End of input */
  unsigned char *z;                     /* Output iterator */
  uint c;

  assert( pMem.flags&MEM_Str );
  assert( pMem.enc!=desiredEnc );
  assert( pMem.enc!=0 );
  assert( pMem.n>=0 );

#if defined(TRANSLATE_TRACE)
  {
    char zBuf[100];
    sqlite3VdbeMemPrettyPrint(pMem, zBuf);
    fprintf(stderr, "INPUT:  %s\n", zBuf);
  }
#endif

  /* If the translation is between UTF-16 little and big endian, then 
  ** all that is required is to swap the byte order. This case is handled
  ** differently from the others.
  */
  if( pMem.enc!=SQLITE_UTF8 && desiredEnc!=SQLITE_UTF8 ){
    byte temp;
    int rc;
    rc = sqlite3VdbeMemMakeWriteable(pMem);
    if( rc!=SQLITE_OK ){
      assert( rc==SQLITE_NOMEM );
      return SQLITE_NOMEM;
    }
    zIn = (byte*)pMem.z;
    zTerm = &zIn[pMem.n&~1];
    while( zIn<zTerm ){
      temp = *zIn;
      *zIn = *(zIn+1);
      zIn++;
      *zIn++ = temp;
    }
    pMem.enc = desiredEnc;
    goto translate_out;
  }

  /* Set len to the maximum number of bytes required in the output buffer. */
  if( desiredEnc==SQLITE_UTF8 ){
    /* When converting from UTF-16, the maximum growth results from
    ** translating a 2-byte character to a 4-byte UTF-8 character.
    ** A single byte is required for the output string
    ** nul-terminator.
    */
    pMem.n &= ~1;
    len = pMem.n * 2 + 1;
  }else{
    /* When converting from UTF-8 to UTF-16 the maximum growth is caused
    ** when a 1-byte UTF-8 character is translated into a 2-byte UTF-16
    ** character. Two bytes are required in the output buffer for the
    ** nul-terminator.
    */
    len = pMem.n * 2 + 2;
  }

  /* Set zIn to point at the start of the input buffer and zTerm to point 1
  ** byte past the end.
  **
  ** Variable zOut is set to point at the output buffer, space obtained
  ** from sqlite3_malloc().
  */
  zIn = (byte*)pMem.z;
  zTerm = &zIn[pMem.n];
  zOut = sqlite3DbMallocRaw(pMem.db, len);
  if( !zOut ){
    return SQLITE_NOMEM;
  }
  z = zOut;

  if( pMem.enc==SQLITE_UTF8 ){
    if( desiredEnc==SQLITE_UTF16LE ){
      /* UTF-8 . UTF-16 Little-endian */
      while( zIn<zTerm ){
        /* c = sqlite3Utf8Read(zIn, zTerm, (const byte**)&zIn); */
        READ_UTF8(zIn, zTerm, c);
        WRITE_UTF16LE(z, c);
      }
    }else{
      assert( desiredEnc==SQLITE_UTF16BE );
      /* UTF-8 . UTF-16 Big-endian */
      while( zIn<zTerm ){
        /* c = sqlite3Utf8Read(zIn, zTerm, (const byte**)&zIn); */
        READ_UTF8(zIn, zTerm, c);
        WRITE_UTF16BE(z, c);
      }
    }
    pMem.n = (int)(z - zOut);
    *z++ = 0;
  }else{
    assert( desiredEnc==SQLITE_UTF8 );
    if( pMem.enc==SQLITE_UTF16LE ){
      /* UTF-16 Little-endian . UTF-8 */
      while( zIn<zTerm ){
        READ_UTF16LE(zIn, zIn<zTerm, c); 
        WRITE_UTF8(z, c);
      }
    }else{
      /* UTF-16 Big-endian . UTF-8 */
      while( zIn<zTerm ){
        READ_UTF16BE(zIn, zIn<zTerm, c); 
        WRITE_UTF8(z, c);
      }
    }
    pMem.n = (int)(z - zOut);
  }
  *z = 0;
  assert( (pMem.n+(desiredEnc==SQLITE_UTF8?1:2))<=len );

  pMem.Release()
  pMem.flags &= ~(MEM_Static|MEM_Dyn|MEM_Ephem);
  pMem.enc = desiredEnc;
  pMem.flags |= (MEM_Term|MEM_Dyn);
  pMem.z = (char*)zOut;
  pMem.zMalloc = pMem.z;

translate_out:
#if defined(TRANSLATE_TRACE)
  {
    char zBuf[100];
    sqlite3VdbeMemPrettyPrint(pMem, zBuf);
    fprintf(stderr, "OUTPUT: %s\n", zBuf);
  }
#endif
  return SQLITE_OK;
}

//	This routine checks for a byte-order mark at the beginning of the UTF-16 string stored in *pMem. If one is present, it is removed and the encoding of the Mem adjusted. This routine does not do any byte-swapping, it just sets Mem.enc appropriately.
//	The allocation (static, dynamic etc.) and encoding of the Mem may be changed by this function.
func (pMem *Mem) HandleBom() (rc int) {
	assert( pMem.n >= 0 )
	var bom byte = 0
	if pMem.n > 1 {
		b1 := ([]byte)(pMem.z)[0]
		b2 := ([]byte)(pMem.z)[1]
		if b1 == 0xFE && b2 == 0xFF {
			bom = SQLITE_UTF16BE
		}
		if b1 == 0xFF && b2 == 0xFE {
			bom = SQLITE_UTF16LE
		}
	}
  
	if bom != 0 {
		if rc = sqlite3VdbeMemMakeWriteable(pMem); rc == SQLITE_OK {
			pMem.n -= 2
			memmove(pMem.z, &pMem.z[2], pMem.n)
			pMem.z[pMem.n] = '\0'
			pMem.z[pMem.n+1] = '\0'
			pMem.flags |= MEM_Term
			pMem.enc = bom
		}
	}
	return
}
#endif /* SQLITE_OMIT_UTF16 */

/*
** pZ is a UTF-8 encoded unicode string. If nByte is less than zero,
** return the number of unicode characters in pZ up to (but not including)
** the first 0x00 byte. If nByte is not less than zero, return the
** number of unicode characters in the first nByte of pZ (or up to 
** the first 0x00, whichever comes first).
*/
 int sqlite3Utf8CharLen(const char *zIn, int nByte){
  int r = 0;
  const byte *z = (const byte*)zIn;
  const byte *zTerm;
  if( nByte>=0 ){
    zTerm = &z[nByte];
  }else{
    zTerm = (const byte*)(-1);
  }
  assert( z<=zTerm );
  while( *z!=0 && z<zTerm ){
    SQLITE_SKIP_UTF8(z);
    r++;
  }
  return r;
}


#ifndef SQLITE_OMIT_UTF16
/*
** Convert a UTF-16 string in the native encoding into a UTF-8 string.
** Memory to hold the UTF-8 string is obtained from sqlite3_malloc and must
** be freed by the calling function.
**
** NULL is returned if there is an allocation error.
*/
 char *sqlite3Utf16to8(sqlite3 *db, const void *z, int nByte, byte enc){
  Mem m;
  memset(&m, 0, sizeof(m));
  m.db = db;
  m.SetStr(z, enc, SQLITE_STATIC)
  sqlite3VdbeChangeEncoding(&m, SQLITE_UTF8);
  if( db.mallocFailed ){
    m.Release()
    m.z = 0;
  }
  assert( (m.flags & MEM_Term)!=0 || db.mallocFailed );
  assert( (m.flags & MEM_Str)!=0 || db.mallocFailed );
  assert( (m.flags & MEM_Dyn)!=0 || db.mallocFailed );
  assert( m.z || db.mallocFailed );
  return m.z;
}

/*
** Convert a UTF-8 string to the UTF-16 encoding specified by parameter
** enc. A pointer to the new string is returned, and the value of *pnOut
** is set to the length of the returned string in bytes.
** 
** If a malloc failure occurs, NULL is returned and the db.mallocFailed
** flag set.
*/
 char *sqlite3Utf8to16(sqlite3 *db, byte enc, char *z, int n, int *pnOut){
  Mem m;
  memset(&m, 0, sizeof(m));
  m.db = db;
  m.SetStr(z, SQLITE_UTF8, SQLITE_STATIC)
  if( sqlite3VdbeMemTranslate(&m, enc) ){
    assert( db.mallocFailed );
    return 0;
  }
  assert( m.z==m.zMalloc );
  *pnOut = m.n;
  return m.z;
}

/*
** zIn is a UTF-16 encoded unicode string at least nChar characters long.
** Return the number of bytes in the first nChar unicode characters
** in pZ.  nChar must be non-negative.
*/
 int sqlite3Utf16ByteLen(const void *zIn, int nChar){
  int c;
  unsigned char const *z = zIn;
  int n = 0;
  
  if( SQLITE_UTF16NATIVE==SQLITE_UTF16BE ){
    while( n<nChar ){
      READ_UTF16BE(z, 1, c);
      n++;
    }
  }else{
    while( n<nChar ){
      READ_UTF16LE(z, 1, c);
      n++;
    }
  }
  return (int)(z-(unsigned char const *)zIn);
}
#endif /* SQLITE_OMIT_UTF16 */
