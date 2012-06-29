/* This file contains functions used to access the internal hash tables
** of user defined functions and collation sequences.
*/


/*
** Invoke the 'collation needed' callback to request a collation sequence
** in the encoding enc of name Name, length nName.
*/
static void callCollNeeded(sqlite3 *db, int enc, const char *Name){
  assert( !db.xCollNeeded || !db.xCollNeeded16 );
  if( db.xCollNeeded ){
    char *zExternal = sqlite3DbStrDup(db, Name);
    if( !zExternal ) return;
    db.xCollNeeded(db.pCollNeededArg, db, enc, zExternal);
    zExternal = nil
  }
#ifndef SQLITE_OMIT_UTF16
  if( db.xCollNeeded16 ){
    char const *zExternal;
    pTmp := db.NewValue()
    sqlite3ValueSetStr(pTmp, -1, Name, SQLITE_UTF8, SQLITE_STATIC);
    zExternal = sqlite3ValueText(pTmp, SQLITE_UTF16NATIVE);
    if( zExternal ){
      db.xCollNeeded16(db.pCollNeededArg, db, int(db.Encoding()), zExternal);
    }
    pTmp.Free()
  }
#endif
}

/*
** This routine is called if the collation factory fails to deliver a
** collation function in the best encoding but there may be other versions
** of this collation function (for other text encodings) available. Use one
** of these instead if they exist. Avoid a UTF-8 <. UTF-16 conversion if
** possible.
*/
static int synthCollSeq(sqlite3 *db, CollSeq *pColl){
  CollSeq *pColl2;
  char *z = pColl.Name;
  int i;
  static const byte aEnc[] = { SQLITE_UTF16BE, SQLITE_UTF16LE, SQLITE_UTF8 };
  for(i=0; i<3; i++){
    pColl2 = sqlite3FindCollSeq(db, aEnc[i], z, 0);
    if( pColl2.xCmp!=0 ){
      memcpy(pColl, pColl2, sizeof(CollSeq));
      pColl.xDel = 0;         /* Do not copy the destructor */
      return SQLITE_OK;
    }
  }
  return SQLITE_ERROR;
}

/*
** This function is responsible for invoking the collation factory callback
** or substituting a collation sequence of a different encoding when the
** requested collation sequence is not available in the desired encoding.
** 
** If it is not NULL, then pColl must point to the database native encoding 
** collation sequence with name Name, length nName.
**
** The return value is either the collation sequence to be used in database
** db for collation type name Name, length nName, or NULL, if no collation
** sequence can be found.
**
** See also: LocateCollSeq(), sqlite3FindCollSeq()
*/
 CollSeq *sqlite3GetCollSeq(
  sqlite3* db,          /* The database connection */
  byte enc,               /* The desired encoding for the collating sequence */
  CollSeq *pColl,       /* Collating sequence with native encoding, or NULL */
  const char *Name     /* Collating sequence name */
){
  CollSeq *p;

  p = pColl;
  if( !p ){
    p = sqlite3FindCollSeq(db, enc, Name, 0);
  }
  if( !p || !p.xCmp ){
    /* No collation sequence of this type for this encoding is registered.
    ** Call the collation factory to see if it can supply us with one.
    */
    callCollNeeded(db, enc, Name);
    p = sqlite3FindCollSeq(db, enc, Name, 0);
  }
  if( p && !p.xCmp && synthCollSeq(db, p) ){
    p = 0;
  }
  assert( !p || p.xCmp );
  return p;
}

/*
** This routine is called on a collation sequence before it is used to
** check that it is defined. An undefined collation sequence exists when
** a database is loaded that contains references to collation sequences
** that have not been defined by sqlite3_create_collation() etc.
**
** If required, this routine calls the 'collation needed' callback to
** request a definition of the collating sequence. If this doesn't work, 
** an equivalent collating sequence that uses a text encoding different
** from the main database is substituted, if one is available.
*/
 int sqlite3CheckCollSeq(Parse *pParse, CollSeq *pColl){
  if( pColl ){
    const char *Name = pColl.Name;
    sqlite3 *db = pParse.db;
    CollSeq *p = sqlite3GetCollSeq(db, db.Encoding(), pColl, Name);
    if( !p ){
      pParse.SetErrorMsg("no such collation sequence: %v", Name);
      pParse.nErr++;
      return SQLITE_ERROR;
    }
    assert( p==pColl );
  }
  return SQLITE_OK;
}



/*
** Locate and return an entry from the db.Collations hash table. If the entry
** specified by Name and nName is not found and parameter 'create' is
** true, then create a new entry. Otherwise return NULL.
**
** Each pointer stored in the sqlite3.Collations hash table contains an
** array of three CollSeq structures. The first is the collation sequence
** prefferred for UTF-8, the second UTF-16le, and the third UTF-16be.
**
** Stored immediately after the three collation sequences is a copy of
** the collation sequence name. A pointer to this string is stored in
** each collation sequence structure.
*/
static CollSeq *findCollSeqEntry(
  sqlite3 *db,          /* Database connection */
  const char *Name,    /* Name of the collating sequence */
  int create            /* Create a new entry if true */
){
	if pColl := db.Collations[Name]; pColl == nil && create {
		db.Collations[Name] = []*CollSeq{
			&CollSeq{ Name: Name, enc: SQLITE_UTF8 },
			&CollSeq{ Name: Name, enc: SQLITE_UTF16LE },
			&CollSeq{ Name: Name, enc: SQLITE_UTF16BE },
		}
	}
	return pColl
}

/*
** Parameter Name points to a UTF-8 encoded string nName bytes long.
** Return the CollSeq* pointer for the collation sequence named Name
** for the encoding 'enc' from the database 'db'.
**
** If the entry specified is not found and 'create' is true, then create a
** new entry.  Otherwise return NULL.
**
** A separate function LocateCollSeq() is a wrapper around
** this routine.  LocateCollSeq() invokes the collation factory
** if necessary and generates an error message if the collating sequence
** cannot be found.
**
** See also: LocateCollSeq(), sqlite3GetCollSeq()
*/
 CollSeq *sqlite3FindCollSeq(
  sqlite3 *db,
  byte enc,
  const char *Name,
  int create
){
  CollSeq *pColl;
  if( Name ){
    pColl = findCollSeqEntry(db, Name, create);
  }else{
    pColl = db.pDfltColl;
  }
  assert( SQLITE_UTF8==1 && SQLITE_UTF16LE==2 && SQLITE_UTF16BE==3 );
  assert( enc>=SQLITE_UTF8 && enc<=SQLITE_UTF16BE );
  if( pColl ) pColl += enc-1;
  return pColl;
}

//	During the search for the best function definition, this procedure is called to test how well the function passed as the first argument matches the request for a function with nArg arguments in a system that uses encoding enc. The value returned indicates how well the request is matched. A higher value indicates a better match.
//	If nArg is -1 that means to only return a match (non-zero) if p.nArg is also -1. In other words, we are searching for a function that takes a variable number of arguments.
//	If nArg is -2 that means that we are searching for any function regardless of the number of arguments it uses, so return a positive match score for any
//	The returned value is always between 0 and 6, as follows:
//		0: Not a match.
//		1: UTF8/16 conversion required and function takes any number of arguments.
//		2: UTF16 byte order change required and function takes any number of args.
//		3: encoding matches and function takes any number of arguments
//		4: UTF8/16 conversion required - argument count matches exactly
//		5: UTF16 byte order conversion required - argument count matches exactly
//		6: Perfect match:  encoding and argument count match exactly.
//	If nArg == -2 then any function with a non-null xStep or xFunc is a perfect match and any function with both xStep and xFunc NULL is a non-match.

#define FUNC_PERFECT_MATCH 6				//	The score for a perfect match

func (p *FuncDef) matchQuality(nArg int, enc byte) (match int) {
static int matchQuality(
  FuncDef *p,     /* The function we are evaluating for match quality */
  int nArg,       /* Desired number of arguments.  (-1)==any */
  byte enc          /* Desired text encoding */
){
	switch {
	case nArg == -2:
		//	nArg of -2 is a special case
		if p.xFunc == nil && p.xStep == nil {
			return 0
		} else {
			return FUNC_PERFECT_MATCH
		}
	case p.nArg != nArg && p.nArg >= 0:
		//	Wrong number of arguments means "no match"
		return 0
	default:
		//	Give a better score to a function with a specific number of arguments than to function that accepts any number of arguments.
		if p.nArg == nArg {
			match = 4
		} else {
			match = 1
		}

		//	Bonus points if the text encoding matches
		if enc == p.iPrefEnc {
			match += 2						//	Exact encoding match
		} else if (enc & p.iPrefEnc & 2) != 0 {
			match += 1						//	Both are UTF16, but with different byte orders
		}
	}
	return
}

//	Search a FuncDefHash for a function with the given name. Return a pointer to the matching FuncDef if found, or nil if there is no match.
func (pHash *FunctionTable) Search(Name string) (p *FuncDef) {
	return pHash[strings.ToLower(p.Name)]
}

//	Insert a new FuncDef into a FunctionTable.
func (f *FunctionTable) Insert(pDef *FuncDef) {
	f[strings.ToLower(pDef.Name)] = pDef
}
  
  
//	Locate a user function given a name, a number of arguments and a flag indicating whether the function prefers UTF-16 over UTF-8. Return a pointer to the FuncDef structure that defines that function, or return NULL if the function does not exist.
//	If the createFunction argument is true, then a new (blank) FuncDef structure is created and liked into the "db" structure if a no matching function previously existed.
//	If nArg is -2, then the first valid function found is returned. A function is valid if either xFunc or xStep is non-zero. The nArg == -2 case is used to see if Name is a valid function name for some number of arguments. If nArg is -2, then createFunction must be 0.
//	If createFunction is false, then a function with the required name and number of arguments may be returned even if the eTextRep flag does not match that requested.
func (db *sqlite3) FindFunction(Name string, nArg int, enc byte, createFunction bool) (pBest *FuncDef) {
	assert( nArg >= -2 )
	assert( nArg >= -1 || !createFunction )
	assert( enc == SQLITE_UTF8 || enc == SQLITE_UTF16LE || enc == SQLITE_UTF16BE )

	//	First search for a match amongst the application-defined functions.
	p := db.aFunc.Search(Name)
	bestScore := 0
	if score := matchQuality(p, nArg, enc); score > bestScore {
		pBest = p
		bestScore = score
	}

	if createFunction {
		//	If the search did not reveal an exact match for the name, number of arguments and encoding, then add a new entry to the hash table and return it.
		if bestScore < FUNC_PERFECT_MATCH {
			pBest = &FuncDef{ Name: Name, nArg: uint16(nArg), iPrefEnc: enc }
			db.aFunc.Insert(pBest)
		}
	} else {
		//	If no match is found, search the built-in functions.
		//	If the SQLITE_PreferBuiltin flag is set, then search the built-in functions even if a prior app-defined function was found. And give priority to built-in functions.
		//	Except, if createFunction is true, that means that we are trying to install a new function. Whatever FuncDef structure is returned it will have fields overwritten with new information appropriate for the new function. But the FuncDefs for built-in functions are read-only. So we must not search for built-ins when creating a new function.
		if pBest == nil || (db.flags & SQLITE_PreferBuiltin) != 0 {
			bestScore = 0
			p = sqlite3GlobalFunctions.Search(Name)
			if score := p.matchQuality(nArg, enc); score > bestScore {
				pBest = p
				bestScore = score
			}
		}
	}

	if pBest != nil && !(pBest.xStep || pBest.xFunc || createFunction) {
		pBest = nil
	}
	return
}

//	Free all resources held by the schema structure. The void* argument points at a Schema struct. This function cleans up subsidiary resources (i.e. the contents of the schema hash tables).
//	The Schema.cache_size variable is not cleared.
func ClearSchema(p interface{}) {
	schema := p.(*Schema)
	schema.Indices = make(map[string]*Index)

	temp2 := schema.Triggers
	schema.Triggers = make(map[string]*Trigger)
	for _, t := range temp2 {
		(*sqlite3)(nil).DeleteTrigger(t)
	}

	temp1 := schema.Tables
	schema.Tables = make(map[string]*Table)
	for _, t := range temp1 {
		(*sqlite3)(nil).DeleteTable(0, t)
	}

	schema.ForeignKeys = make(map[string]*ForeignKey)
	schema.pSeqTab = nil
	if schema.flags & DB_SchemaLoaded {
		schema.iGeneration++
		schema.flags &= ~DB_SchemaLoaded
	}
}

//	Find and return the schema associated with a BTree. Create a new one if necessary.
func (db *sqlite3) GetSchema(pBt *Btree) (p *Schema) {
	if pBt != nil {
		p = pBt.Schema(true, ClearSchema)
	} else {
		p = &Schema{}
	}
	if p == nil {
		db.mallocFailed = true
	} else if p.file_format == 0 {
		p.Tables = make(map[string]*Table)
		p.Indices = make(map[string]*Index)
		p.Triggers = make(map[string]*Trigger)
		p.ForeignKeys = make(map[string]*ForeignKey)
		p.enc = SQLITE_UTF8
	}
	return
}
