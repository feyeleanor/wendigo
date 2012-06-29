//	Conversion types fall into various categories as defined by the following enumeration.
#define etRADIX       1 /* Integer types.  %d, %x, %o, and so forth */
#define etFLOAT       2 /* Floating point.  %f */
#define etEXP         3 /* Exponentional notation. %e and %E */
#define etGENERIC     4 /* Floating or exponential, depending on exponent. %g */
#define etSIZE        5 /* Return number of characters processed so far. %n */
#define etSTRING      6 /* Strings. %s */
#define etDYNSTRING   7 /* Dynamically allocated strings. %z */
#define etPERCENT     8 /* Percent symbol. %% */
#define etCHARX       9 /* Characters. %c */
/* The rest are extensions, not normally found in printf() */
#define etSQLESCAPE  10 /* Strings with '\'' doubled.  %q */
#define etSQLESCAPE2 11 /* Strings with '\'' doubled and enclosed in '',
                          NULL pointers replaced by SQL NULL.  %Q */
#define etTOKEN      12 /* a pointer to a Token structure */
#define etSRCLIST    13 /* a pointer to a SrcList */
#define etPOINTER    14 /* The %p conversion */
#define etSQLESCAPE3 15 /* %w . Strings with '\"' doubled */
#define etORDINAL    16 /* %r . 1st, 2nd, 3rd, 4th, etc.  English only */

#define etINVALID     0 /* Any unrecognized conversion type */


//	An "etByte" is an 8-bit unsigned value.
typedef unsigned char etByte;

//	Each builtin conversion character (ex: the 'd' in "%d") is described by an instance of the following structure
typedef struct et_info {   /* Information about each format field */
  char fmttype;            /* The format field code letter */
  etByte base;             /* The base for radix conversion */
  etByte flags;            /* One or more of FLAG_ constants below */
  etByte type;             /* Conversion paradigm */
  etByte charset;          /* Offset into aDigits[] of the digits string */
  etByte prefix;           /* Offset into aPrefix[] of the prefix string */
} et_info;

//	Allowed values for et_info.flags
#define FLAG_SIGNED  1     /* True if the value to convert is signed */
#define FLAG_INTERN  2     /* True if for internal use only */
#define FLAG_STRING  4     /* Allow infinity precision */


//	The following table is searched linearly, so it is good to put the most frequently used conversion types first.
static const char aDigits[] = "0123456789ABCDEF0123456789abcdef";
static const char aPrefix[] = "-x0\000X0";
static const et_info fmtinfo[] = {
  {  'd', 10, 1, etRADIX,      0,  0 },
  {  's',  0, 4, etSTRING,     0,  0 },
  {  'g',  0, 1, etGENERIC,    30, 0 },
  {  'z',  0, 4, etDYNSTRING,  0,  0 },
  {  'q',  0, 4, etSQLESCAPE,  0,  0 },
  {  'Q',  0, 4, etSQLESCAPE2, 0,  0 },
  {  'w',  0, 4, etSQLESCAPE3, 0,  0 },
  {  'c',  0, 0, etCHARX,      0,  0 },
  {  'o',  8, 0, etRADIX,      0,  2 },
  {  'u', 10, 0, etRADIX,      0,  0 },
  {  'x', 16, 0, etRADIX,      16, 1 },
  {  'X', 16, 0, etRADIX,      0,  4 },
  {  'f',  0, 1, etFLOAT,      0,  0 },
  {  'e',  0, 1, etEXP,        30, 0 },
  {  'E',  0, 1, etEXP,        14, 0 },
  {  'G',  0, 1, etGENERIC,    14, 0 },
  {  'i', 10, 1, etRADIX,      0,  0 },
  {  'n',  0, 0, etSIZE,       0,  0 },
  {  '%',  0, 0, etPERCENT,    0,  0 },
  {  'p', 16, 0, etPOINTER,    0,  1 },

//	All the rest have the FLAG_INTERN bit set and are thus for internal use only
  {  'T',  0, 2, etTOKEN,      0,  0 },
  {  'S',  0, 2, etSRCLIST,    0,  0 },
  {  'r', 10, 3, etORDINAL,    0,  0 },
};

//	Format and write a message to the log if logging is enabled.
func void sqlite3_log(int iErrCode, const char *zFormat, ap... string) {
	if sqlite3GlobalConfig.xLog {
		sqlite3GlobalConfig.xLog(sqlite3GlobalConfig.pLogArg, iErrCode, fmt.Sprintf(zFormat, ap))
	}
}