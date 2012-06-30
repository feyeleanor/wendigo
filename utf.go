/* This file contains routines used by UTF-8.
**
** Notes on UTF-8:
**
**   Byte-0    Byte-1    Byte-2    Byte-3    Value
**  0xxxxxxx                                 00000000 00000000 0xxxxxxx
**  110yyyyy  10xxxxxx                       00000000 00000yyy yyxxxxxx
**  1110zzzz  10yyyyyy  10xxxxxx             00000000 zzzzyyyy yyxxxxxx
**  11110uuu  10uuzzzz  10yyyyyy  10xxxxxx   000uuuuu zzzzyyyy yyxxxxxx
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


//	Translate a single UTF-8 character.  Return the unicode value.
//	During translation, assume that the byte that zTerm points is a 0x00.
//	Write a pointer to the next unread byte back into *pzNext.
//	Notes On Invalid UTF-8:
//		*  This routine never allows a 7-bit character (0x00 through 0x7f) to be encoded as a multi-byte character. Any multi-byte character that attempts to encode a value between 0x00 and 0x7f is rendered as 0xfffd.
//		*  Bytes in the range of 0x80 through 0xbf which occur as the first byte of a character are interpreted as single-byte characters and rendered as themselves even though they are technically invalid characters.
//		*  This routine accepts an infinite number of different UTF8 encodings for unicode values 0x80 and greater. It do not change over-length encodings to 0xfffd as some systems recommend.
uint32 sqlite3Utf8Read(
  const unsigned char *zIn,       /* First byte of UTF-8 character */
  const unsigned char **pzNext    /* Write first byte past UTF-8 char here */
){
	uint c
	//	For this routine, we assume the UTF8 string is always zero-terminated.
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

//	pZ is a UTF-8 encoded unicode string. If nByte is less than zero, return the number of unicode characters in pZ up to (but not including) the first 0x00 byte. If nByte is not less than zero, return the number of unicode characters in the first nByte of pZ (or up to the first 0x00, whichever comes first).
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