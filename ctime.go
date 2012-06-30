/* This file implements routines used to report what compile-time options
** SQLite was built with.
*/

#ifndef SQLITE_OMIT_COMPILEOPTION_DIAGS


/*
** An array of names of all compile-time options.  This array should 
** be sorted A-Z.
**
** This array looks large, but in a typical installation actually uses
** only a handful of compile-time options, so most times this array is usually
** rather short and uses little memory space.
*/
static const char * const azCompileOpt[] = {

/* These macros are provided to "stringify" the value of the define
** for those options in which the value is meaningful. */
#define CTIMEOPT_VAL_(opt) #opt
#define CTIMEOPT_VAL(opt) CTIMEOPT_VAL_(opt)

#ifdef SQLITE_32BIT_ROWID
  "32BIT_ROWID",
#endif
#ifdef SQLITE_4_BYTE_ALIGNED_MALLOC
  "4_BYTE_ALIGNED_MALLOC",
#endif
#ifdef SQLITE_CASE_SENSITIVE_LIKE
  "CASE_SENSITIVE_LIKE",
#endif
#ifdef SQLITE_CHECK_PAGES
  "CHECK_PAGES",
#endif
#ifdef SQLITE_DEFAULT_LOCKING_MODE
  "DEFAULT_LOCKING_MODE=" CTIMEOPT_VAL(SQLITE_DEFAULT_LOCKING_MODE),
#endif
#ifdef SQLITE_DISABLE_DIRSYNC
  "DISABLE_DIRSYNC",
#endif
#ifdef SQLITE_ENABLE_ATOMIC_WRITE
  "ENABLE_ATOMIC_WRITE",
#endif
#ifdef SQLITE_ENABLE_CEROD
  "ENABLE_CEROD",
#endif
#ifdef SQLITE_ENABLE_COLUMN_METADATA
  "ENABLE_COLUMN_METADATA",
#endif
#ifdef SQLITE_ENABLE_EXPENSIVE_ASSERT
  "ENABLE_EXPENSIVE_ASSERT",
#endif
#ifdef SQLITE_ENABLE_FTS1
  "ENABLE_FTS1",
#endif
#ifdef SQLITE_ENABLE_FTS2
  "ENABLE_FTS2",
#endif
#ifdef SQLITE_ENABLE_FTS3
  "ENABLE_FTS3",
#endif
#ifdef SQLITE_ENABLE_FTS3_PARENTHESIS
  "ENABLE_FTS3_PARENTHESIS",
#endif
#ifdef SQLITE_ENABLE_FTS4
  "ENABLE_FTS4",
#endif
#ifdef SQLITE_ENABLE_IOTRACE
  "ENABLE_IOTRACE",
#endif
#ifdef SQLITE_ENABLE_LOAD_EXTENSION
  "ENABLE_LOAD_EXTENSION",
#endif
#ifdef SQLITE_ENABLE_MEMORY_MANAGEMENT
  "ENABLE_MEMORY_MANAGEMENT",
#endif
#ifdef SQLITE_ENABLE_OVERSIZE_CELL_CHECK
  "ENABLE_OVERSIZE_CELL_CHECK",
#endif
#ifdef SQLITE_ENABLE_RTREE
  "ENABLE_RTREE",
#endif
  "ENABLE_STAT3",
  "ENABLE_UNLOCK_NOTIFY",
#ifdef SQLITE_ENABLE_UPDATE_DELETE_LIMIT
  "ENABLE_UPDATE_DELETE_LIMIT",
#endif
#ifdef SQLITE_HAVE_ISNAN
  "HAVE_ISNAN",
#endif
#ifdef SQLITE_HOMEGROWN_RECURSIVE_MUTEX
  "HOMEGROWN_RECURSIVE_MUTEX",
#endif
#ifdef SQLITE_IGNORE_AFP_LOCK_ERRORS
  "IGNORE_AFP_LOCK_ERRORS",
#endif
#ifdef SQLITE_IGNORE_FLOCK_LOCK_ERRORS
  "IGNORE_FLOCK_LOCK_ERRORS",
#endif
#ifdef SQLITE_LOCK_TRACE
  "LOCK_TRACE",
#endif
#ifdef SQLITE_MAX_SCHEMA_RETRY
  "MAX_SCHEMA_RETRY=" CTIMEOPT_VAL(SQLITE_MAX_SCHEMA_RETRY),
#endif
#ifdef SQLITE_NO_SYNC
  "NO_SYNC",
#endif
#ifdef SQLITE_OMIT_ALTERTABLE
  "OMIT_ALTERTABLE",
#endif
#ifdef SQLITE_OMIT_ATTACH
  "OMIT_ATTACH",
#endif
#ifdef SQLITE_OMIT_AUTOINCREMENT
  "OMIT_AUTOINCREMENT",
#endif
  "OMIT_AUTOINIT",
#ifdef SQLITE_OMIT_AUTORESET
  "OMIT_AUTORESET",
#endif
#ifdef SQLITE_OMIT_BETWEEN_OPTIMIZATION
  "OMIT_BETWEEN_OPTIMIZATION",
#endif
  "OMIT_BUILTIN_TEST",
#ifdef SQLITE_OMIT_CAST
  "OMIT_CAST",
#endif
/* // redundant
** #ifdef SQLITE_OMIT_COMPILEOPTION_DIAGS
**   "OMIT_COMPILEOPTION_DIAGS",
** #endif
*/
#ifdef SQLITE_OMIT_COMPLETE
  "OMIT_COMPLETE",
#endif
#ifdef SQLITE_OMIT_DATETIME_FUNCS
  "OMIT_DATETIME_FUNCS",
#endif
#ifdef SQLITE_OMIT_DECLTYPE
  "OMIT_DECLTYPE",
#endif
  "OMIT_DEPRECATED",
#ifdef SQLITE_OMIT_EXPLAIN
  "OMIT_EXPLAIN",
#endif
#ifdef SQLITE_OMIT_FLAG_PRAGMAS
  "OMIT_FLAG_PRAGMAS",
#endif
#ifdef SQLITE_OMIT_GET_TABLE
  "OMIT_GET_TABLE",
#endif
#ifdef SQLITE_OMIT_LIKE_OPTIMIZATION
  "OMIT_LIKE_OPTIMIZATION",
#endif
#ifdef SQLITE_OMIT_LOAD_EXTENSION
  "OMIT_LOAD_EXTENSION",
#endif
#ifdef SQLITE_OMIT_LOCALTIME
  "OMIT_LOCALTIME",
#endif
  "OMIT_LOOKASIDE",
#ifdef SQLITE_OMIT_MEMORYDB
  "OMIT_MEMORYDB",
#endif
#ifdef SQLITE_OMIT_OR_OPTIMIZATION
  "OMIT_OR_OPTIMIZATION",
#endif
#ifdef SQLITE_OMIT_PAGER_PRAGMAS
  "OMIT_PAGER_PRAGMAS",
#endif
#ifdef SQLITE_OMIT_PRAGMA
  "OMIT_PRAGMA",
#endif
#ifdef SQLITE_OMIT_PROGRESS_CALLBACK
  "OMIT_PROGRESS_CALLBACK",
#endif
#ifdef SQLITE_OMIT_SCHEMA_PRAGMAS
  "OMIT_SCHEMA_PRAGMAS",
#endif
#ifdef SQLITE_OMIT_SCHEMA_VERSION_PRAGMAS
  "OMIT_SCHEMA_VERSION_PRAGMAS",
#endif
#ifdef SQLITE_OMIT_TCL_VARIABLE
  "OMIT_TCL_VARIABLE",
#endif
#ifdef SQLITE_OMIT_TRACE
  "OMIT_TRACE",
#endif
#ifdef SQLITE_OMIT_TRUNCATE_OPTIMIZATION
  "OMIT_TRUNCATE_OPTIMIZATION",
#endif
  "OMIT_UTF16",
#ifdef SQLITE_OMIT_VACUUM
  "OMIT_VACUUM",
#endif
#ifdef SQLITE_OMIT_WAL
  "OMIT_WAL",
#endif
#ifdef SQLITE_PERFORMANCE_TRACE
  "PERFORMANCE_TRACE",
#endif
#ifdef SQLITE_PROXY_DEBUG
  "PROXY_DEBUG",
#endif
#ifdef SQLITE_SECURE_DELETE
  "SECURE_DELETE",
#endif
#ifdef SQLITE_SOUNDEX
  "SOUNDEX",
#endif
#ifdef SQLITE_TCL
  "TCL",
#endif
#ifdef SQLITE_TEMP_STORE
  "TEMP_STORE=" CTIMEOPT_VAL(SQLITE_TEMP_STORE),
#endif
#ifdef SQLITE_USE_ALLOCA
  "USE_ALLOCA",
#endif
};

/*
** Given the name of a compile-time option, return true if that option
** was used and false if not.
**
** The name can optionally begin with "SQLITE_" but the "SQLITE_" prefix
** is not required for a match.
*/
func int sqlite3_compileoption_used(const char *zOptName){
  int i, n;
  if CaseInsensitiveMatchN(zOptName, "SQLITE_", 7) {
	  zOptName += 7
	}
  n = sqlite3Strlen30(zOptName);

  /* Since ArraySize(azCompileOpt) is normally in single digits, a
  ** linear search is adequate.  No need for a binary search. */
  for(i=0; i<ArraySize(azCompileOpt); i++){
    if CaseInsensitiveMatchN(zOptName, azCompileOpt[i], n) && ( (azCompileOpt[i][n]==0) || (azCompileOpt[i][n]=='=') ) {
		return 1
	}
  }
  return 0;
}

/*
** Return the N-th compile-time option string.  If N is out of range,
** return a NULL pointer.
*/
func const char *sqlite3_compileoption_get(int N){
  if( N>=0 && N<ArraySize(azCompileOpt) ){
    return azCompileOpt[N];
  }
  return 0;
}

#endif /* SQLITE_OMIT_COMPILEOPTION_DIAGS */
