/* This file contains the C functions that implement mutexes.
**
** This implementation in this file does not provide any mutual
** exclusion and is thus suitable for use only in applications
** that use SQLite in a single thread.  The routines defined
** here are place-holders.  Applications can substitute working
** mutex routines at start-time using the
**
**     sqlite3_config(SQLITE_CONFIG_MUTEX,...)
**
** interface.
*/

//	If compiled with SQLITE_MUTEX_NOOP, then the no-op mutex implementation is used regardless of the run-time threadsafety setting.
#ifdef SQLITE_MUTEX_NOOP
 sqlite3_mutex_methods const *sqlite3DefaultMutex(void){
  return sqlite3NoopMutex();
}
#endif /* defined(SQLITE_MUTEX_NOOP) */