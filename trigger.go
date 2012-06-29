//	This file contains the implementation for TRIGGERs

//	Each trigger present in the database schema is stored as an instance of struct Trigger. 
//	Pointers to instances of struct Trigger are stored in two ways.
//		1. In the "Triggers" hash table (part of the sqlite3* that represents the database). This allows Trigger structures to be retrieved by name.
//		2. All triggers associated with a single table form a linked list, using the Next member of struct Trigger. A pointer to the first element of the linked list is stored as the "pTrigger" member of the associated struct Table.
//	The "Steps" member points to the first element of a linked list containing the SQL statements specified as the trigger program.

type Trigger struct {
	Name		string			//	The name of the trigger
	Table		string			//	The table or view to which the trigger applies
	OP			byte			//	One of TK_DELETE, TK_UPDATE, TK_INSERT
	Time		byte			//	One of TRIGGER_BEFORE, TRIGGER_AFTER
	When		Expr			//	The WHEN clause of the expression (may be NULL)
	Columns		*IdList			//	If this is an UPDATE OF <column-list> trigger, the <column-list> is stored here
	*Schema						//	Schema containing the trigger
	TableSchema	*Schema			//	Schema containing the table
	Steps		*TriggerStep	//	Link list of trigger program steps
	Next		*Trigger		//	Next trigger associated with the table
}

//	A trigger is either a BEFORE or an AFTER trigger. The following constants determine which. 
//	If there are multiple triggers, you might of some BEFORE and some AFTER.
//	In that cases, the constants below can be ORed together.
const(
	TRIGGER_BEFORE = 1
	TRIGGER_AFTER = 2
)

//	An instance of struct TriggerStep is used to store a single SQL statement that is a part of a trigger-program. 
//	Instances of struct TriggerStep are stored in a singly linked list (linked using the "Next" member) referenced by the "Steps" member of the associated struct Trigger instance. The first element of the linked list is the first step of the trigger-program.
//	The "op" member indicates whether this is a "DELETE", "INSERT", "UPDATE" or "SELECT" statement. The meanings of the other members is determined by the value of "op" as follows:
//
//		(op == TK_INSERT)
//		OnConflict    . stores the ON CONFLICT algorithm
//		pSelect   . If this is an INSERT INTO ... SELECT ... statement, then this stores a pointer to the SELECT statement. Otherwise NULL.
//		target    . A token holding the quoted name of the table to insert into.
//		ExprList . If this is an INSERT INTO ... VALUES ... statement, then this stores values to be inserted. Otherwise NULL.
//		IdList   . If this is an INSERT INTO ... (<column-names>) VALUES ... statement, then this stores the column-names to be inserted into.
//
//		(op == TK_DELETE)
//		target    . A token holding the quoted name of the table to delete from.
//		pWhere    . The WHERE clause of the DELETE statement if one is specified. Otherwise NULL.
//
//		(op == TK_UPDATE)
//		target    . A token holding the quoted name of the table to update rows of.
//		pWhere    . The WHERE clause of the UPDATE statement if one is specified. Otherwise NULL.
//		ExprList . A list of the columns to update and the expressions to update them to. See sqlite3Update() documentation of "pChanges" argument.

type TriggerStep struct {
	OP				byte			//	One of TK_DELETE, TK_UPDATE, TK_INSERT, TK_SELECT
	OnConflict		byte			//	OE_Rollback etc.
	*Trigger						//	The trigger that this step is a part of
	*Select							//	SELECT statment or RHS of INSERT INTO .. SELECT ...
	Target			*String			//	Target table for DELETE, UPDATE, INSERT
	Where			*Expr			//	The WHERE clause for DELETE or UPDATE steps
	*ExprList						//	SET clause for UPDATE.  VALUES clause for INSERT
	*IdList							//	Column names for INSERT
	Next			*TriggerStep
	Last			*TriggerStep	//	Last element in link-list. Valid for 1st elem only
}

//	Delete a linked list of TriggerStep structures.
func (db *sqlite3) DeleteTriggerStep(p *TriggerStep) {
	for p != nil {
		pTmp = p
		p = p.Next

		db.ExprDelete(pTmp.Where)
		db.ExprListDelete(pTmp.ExprList)
		sqlite3SelectDelete(db, pTmp.Select)
		sqlite3IdListDelete(db, pTmp.IdList)
	}
}

//	Given table pTab, return a list of all the triggers attached to the table. The list is connected by Trigger.Next pointers.
//	All of the triggers on pTab that are in the same database as pTab are already attached to pTab.pTrigger. But there might be additional triggers on pTab in the TEMP schema. This routine prepends all TEMP triggers on pTab to the beginning of the pTab.pTrigger list and returns the combined list.
//	To state it another way: This routine returns a list of all triggers that fire off of pTab. The list will include any TEMP triggers on pTab as well as the triggers lised in pTab.pTrigger.
func (pParse *Parse) TriggerList(pTab *Table) (pList *Trigger) {
	if !pParse.disableTriggers {
		if schema := pParse.db.Databases[1].Schema; schema != pTab.Schema {
			for  _, pTrig := schema.Triggers {
				if pTrig.pTableSchema == pTab.Schema && CaseInsensitiveMatch(pTrig.table, pTab.Name) {
					if pList == nil {
						pTrig.Next = pTab.pTrigger
					} else {
						pTrig.Next = pList
					}
					pList = pTrig
				}
			}
		}
		if pList == nil {
			pList = pTab.pTrigger
		}
	}
	return
}

//	This is called by the parser when it sees a CREATE TRIGGER statement up to the point of the BEGIN before the trigger actions. A Trigger structure is generated based on the information available and stored in pParse.pNewTrigger. After the trigger actions have been parsed, the sqlite3FinishTrigger() function is called to complete the trigger construction process.

void sqlite3BeginTrigger(
  Parse *pParse,      /* The parse context of the CREATE TRIGGER statement */
  Token *pName1,      /* The name of the trigger */
  Token *pName2,      /* The name of the trigger */
  int Time,          /* One of TK_BEFORE, TK_AFTER, TK_INSTEAD */
  int op,             /* One of TK_INSERT, TK_UPDATE, TK_DELETE */
  IdList *pColumns,   /* column list if this is an UPDATE OF trigger */
  SrcList *pTableName,/* The name of the table/view the trigger applies to */
  Expr *pWhen,        /* WHEN clause */
  int isTemp,         /* True if the TEMPORARY keyword is present */
  int noErr           /* Suppress errors if the trigger already exists */
){
  Trigger *pTrigger = 0;  /* The new trigger */
  Table *pTab;            /* Table that the trigger fires off of */
  char *Name = 0;        /* Name of the trigger */
  sqlite3 *db = pParse.db;  /* The database connection */
  int iDb;                /* The database to store the trigger in */
  Token *pName;           /* The unqualified db name */
  int iTabDb;             /* Index of the database holding pTab */

  assert( pName1!=0 );   /* pName1.z might be NULL, but not pName1 itself */
  assert( pName2!=0 );
  assert( op==TK_INSERT || op==TK_UPDATE || op==TK_DELETE );
  assert( op>0 && op<0xff );
  if( isTemp ){
    /* If TEMP was specified, then the trigger name may not be qualified. */
    if( pName2.n>0 ){
      pParse.SetErrorMsg("temporary trigger may not have qualified name");
      goto trigger_cleanup;
    }
    iDb = 1;
    pName = pName1;
  }else{
    /* Figure out the db that the the trigger will be created in */
    pName, iDb = pParse.TwoPartName(pName1, pName2)
    if( iDb<0 ){
      goto trigger_cleanup;
    }
  }
  if( !pTableName || db.mallocFailed ){
    goto trigger_cleanup;
  }

  /* A long-standing parser bug is that this syntax was allowed:
  **
  **    CREATE TRIGGER attached.demo AFTER INSERT ON attached.tab ....
  **                                                 ^^^^^^^^
  **
  ** To maintain backwards compatibility, ignore the database
  ** name on pTableName if we are reparsing our of SQLITE_MASTER.
  */
  if( db.init.busy && iDb!=1 ){
    pTableName.a[0].zDatabase = nil
  }

  /* If the trigger name was unqualified, and the table is a temp table,
  ** then set iDb to 1 to create the trigger in the temporary database.
  ** If SrcListLookup() returns 0, indicating the table does not
  ** exist, the error is caught by the block below.
  */
  pTab = pParse.SrcListLookup(pTableName)
  if( db.init.busy==0 && pName2.n==0 && pTab
        && pTab.Schema==db.Databases[1].Schema ){
    iDb = 1;
  }

  /* Ensure the table name matches database name and that the table exists */
  if( db.mallocFailed ) goto trigger_cleanup;
  assert( pTableName.nSrc==1 );
  if sFix := p.Parse.NewDbFixer(iDb, "trigger", pName); sFix != nil && sFix.FixSrcList(pTableName) != SQLITE_OK {
    goto trigger_cleanup;
  }
  pTab = pParse.SrcListLookup(pTableName)
  if( !pTab ){
    /* The table does not exist. */
    if( db.init.iDb==1 ){
      /* Ticket #3810.
      ** Normally, whenever a table is dropped, all associated triggers are
      ** dropped too.  But if a TEMP trigger is created on a non-TEMP table
      ** and the table is dropped by a different database connection, the
      ** trigger is not visible to the database connection that does the
      ** drop so the trigger cannot be dropped.  This results in an
      ** "orphaned trigger" - a trigger whose associated table is missing.
      */
      db.init.orphanTrigger = 1;
    }
    goto trigger_cleanup;
  }
  if( pTab.IsVirtual() ){
    pParse.SetErrorMsg("cannot create triggers on virtual tables");
    goto trigger_cleanup;
  }

  //	Check that the trigger name is not reserved and that no trigger of the specified name exists
  Name = Dequote(pName)
  if( !Name || SQLITE_OK!=sqlite3CheckObjectName(pParse, Name) ){
    goto trigger_cleanup;
  }
  if db.Databases[iDb].Schema.Triggers[Name] {
    if( !noErr ){
      pParse.SetErrorMsg("trigger %v already exists", pName)
    }else{
      assert( !db.init.busy );
      pParse.CodeVerifySchema(iDb)
    }
    goto trigger_cleanup;
  }

  /* Do not create a trigger on a system table */
  if CaseInsensitiveMatchN(pTab.Name, "sqlite_", 7) {
    pParse.SetErrorMsg("cannot create trigger on system table");
    pParse.nErr++;
    goto trigger_cleanup;
  }

  /* INSTEAD of triggers are only for views and views only support INSTEAD
  ** of triggers.
  */
  if( pTab.Select && Time!=TK_INSTEAD ){
    pParse"cannot create %v trigger on view: %v", (Time == TK_BEFORE)?"BEFORE":"AFTER", pTableName);
    goto trigger_cleanup;
  }
  if( !pTab.Select && Time==TK_INSTEAD ){
    pParse.SetErrorMsg("cannot create INSTEAD OF trigger on table: %v", pTableName);
    goto trigger_cleanup;
  }
  iTabDb = db.SchemaToIndex(pTab.Schema)

  {
    int code = SQLITE_CREATE_TRIGGER;
    const char *zDb = db.Databases[iTabDb].Name;
    const char *zDbTrig = isTemp ? db.Databases[1].Name : zDb;
    if( iTabDb==1 || isTemp ) code = SQLITE_CREATE_TEMP_TRIGGER;
    if pParse.AuthCheck(code, Name, pTab.Name, zDbTrig) != SQLITE_OK {
      goto trigger_cleanup;
    }
    if pParse.AuthCheck(SQLITE_INSERT, SCHEMA_TABLE(iTabDb),0,zDb) != SQLITE_OK {
      goto trigger_cleanup;
    }
  }

  /* INSTEAD OF triggers can only appear on views and BEFORE triggers
  ** cannot appear on views.  So we might as well translate every
  ** INSTEAD OF trigger into a BEFORE trigger.  It simplifies code
  ** elsewhere.
  */
  if (Time == TK_INSTEAD){
    Time = TK_BEFORE;
  }

  /* Build the Trigger object */
  pTrigger = (Trigger*)sqlite3DbMallocZero(db, sizeof(Trigger));
  if( pTrigger==0 ) goto trigger_cleanup;
  pTrigger.Name = Name;
  Name = 0;
  pTrigger.table = sqlite3DbStrDup(db, pTableName.a[0].Name);
  pTrigger.Schema = db.Databases[iDb].Schema;
  pTrigger.pTableSchema = pTab.Schema;
  pTrigger.op = (byte)op;
  pTrigger.Time = Time==TK_BEFORE ? TRIGGER_BEFORE : TRIGGER_AFTER;
  pTrigger.pWhen = pWhen.Dup();
  pTrigger.pColumns = pColumns.Dup()
  assert( pParse.pNewTrigger==0 );
  pParse.pNewTrigger = pTrigger;

trigger_cleanup:
  Name = nil
  sqlite3SrcListDelete(db, pTableName);
  sqlite3IdListDelete(db, pColumns);
  db.ExprDelete(pWhen)
  if( !pParse.pNewTrigger ){
    db.DeleteTrigger(pTrigger)
  }else{
    assert( pParse.pNewTrigger==pTrigger );
  }
}

/*
** This routine is called after all of the trigger actions have been parsed
** in order to complete the process of building the trigger.
*/
 void sqlite3FinishTrigger(
  Parse *pParse,          /* Parser context */
  TriggerStep *pStepList, /* The triggered program */
  Token *pAll             /* Token that describes the complete CREATE TRIGGER */
){
  Trigger *pTrig = pParse.pNewTrigger;   /* Trigger being finished */
  char *Name;                            /* Name of trigger */
  sqlite3 *db = pParse.db;               /* The database */
  int iDb;                                /* Database containing the trigger */
  Token nameToken;                        /* Trigger name for error reporting */

  pParse.pNewTrigger = 0;
  if( pParse.nErr || !pTrig ) goto triggerfinish_cleanup;
  Name = pTrig.Name;
  iDb = pParse.db.SchemaToIndex(pTrig.Schema)
  pTrig.Steps = pStepList;
  while( pStepList ){
    pStepList.Trigger = pTrig;
    pStepList = pStepList.Next;
  }
  nameToken.z = pTrig.Name;
  nameToken.n = sqlite3Strlen30(nameToken.z);
  if sFix := pParse.NewDbFixer(iDb, "trigger", nameToken); sFix != nil && sFix.FixTriggerStep(pTrig.Steps) != SQLITE_OK {
    goto triggerfinish_cleanup;
  }

  /* if we are not initializing,
  ** build the sqlite_master entry
  */
  if( !db.init.busy ){
    Vdbe *v;
    char *z;

    /* Make an entry in the sqlite_master table */
    v = pParse.GetVdbe()
    if( v==0 ) goto triggerfinish_cleanup;
    pParse.BeginWriteOperation(0, iDb)
    z = sqlite3DbStrNDup(db, (char*)pAll.z, pAll.n);
    sqlite3NestedParse(pParse,
       "INSERT INTO %Q.%s VALUES('trigger',%Q,%Q,0,'CREATE TRIGGER %q')",
       db.Databases[iDb].Name, SCHEMA_TABLE(iDb), Name,
       pTrig.table, z);
    z = nil
    sqlite3ChangeCookie(pParse, iDb);
    v.AddParseSchemaOp(iDb, fmt.Sprintf("type='trigger' AND name='%v'", Name))
  }

  if( db.init.busy ){
    Trigger *pLink = pTrig;
    db.Databases[iDb].Schema.Triggers[Name] = pTrig
    if pLink.Schema == pLink.pTableSchema {
      Table *pTab;
      int n = sqlite3Strlen30(pLink.table);
      pTab = pLink.pTableSchema.Tables[pLink.table]
      assert( pTab!=0 );
      pLink.Next = pTab.pTrigger;
      pTab.pTrigger = pLink;
    }
  }

triggerfinish_cleanup:
  db.DeleteTrigger(pTrig)
  assert( !pParse.pNewTrigger );
  db.DeleteTriggerStep(pStepList)
}

/*
** Turn a SELECT statement (that the pSelect parameter points to) into
** a trigger step.  Return a pointer to a TriggerStep structure.
**
** The parser calls this routine when it finds a SELECT statement in
** body of a TRIGGER.  
*/
 TriggerStep *sqlite3TriggerSelectStep(sqlite3 *db, Select *pSelect){
  TriggerStep *pTriggerStep = sqlite3DbMallocZero(db, sizeof(TriggerStep));
  if( pTriggerStep==0 ) {
    sqlite3SelectDelete(db, pSelect);
    return 0;
  }
  pTriggerStep.op = TK_SELECT;
  pTriggerStep.Select = pSelect;
  pTriggerStep.OnConflict = OE_Default;
  return pTriggerStep;
}

/*
** Allocate space to hold a new trigger step.  The allocated space
** holds both the TriggerStep object and the TriggerStep.Target string.
**
** If an OOM error occurs, NULL is returned and db.mallocFailed is set.
*/
static TriggerStep *triggerStepAllocate(
  sqlite3 *db,                /* Database connection */
  byte op,                      /* Trigger opcode */
  string *pName                /* The target name */
){
  TriggerStep *pTriggerStep;

  pTriggerStep = sqlite3DbMallocZero(db, sizeof(TriggerStep) + pName.n);
  if( pTriggerStep ){
    pTriggerStep.Target = pName
    pTriggerStep.OP = op;
  }
  return pTriggerStep;
}

/*
** Build a trigger step out of an INSERT statement.  Return a pointer
** to the new trigger step.
**
** The parser calls this routine when it sees an INSERT inside the
** body of a trigger.
*/
 TriggerStep *sqlite3TriggerInsertStep(
  sqlite3 *db,        /* The database connection */
  Token *pTableName,  /* Name of the table into which we insert */
  IdList *pColumn,    /* List of columns in pTableName to insert into */
  ExprList *pEList,   /* The VALUE clause: a list of values to be inserted */
  Select *pSelect,    /* A SELECT statement that supplies values */
  byte OnConflict           /* The conflict algorithm (OE_Abort, OE_Replace, etc.) */
){
  TriggerStep *pTriggerStep;

  assert(pEList == 0 || pSelect == 0);
  assert(pEList != 0 || pSelect != 0 || db.mallocFailed);

  pTriggerStep = triggerStepAllocate(db, TK_INSERT, pTableName);
  if( pTriggerStep ){
    pTriggerStep.Select = pSelect.Dup();
    pTriggerStep.IdList = pColumn;
    pTriggerStep.ExprList = pEList.Dup();
    pTriggerStep.OnConflict = OnConflict;
  }else{
    sqlite3IdListDelete(db, pColumn);
  }
  db.ExprListDelete(pEList);
  sqlite3SelectDelete(db, pSelect);

  return pTriggerStep;
}

/*
** Construct a trigger step that implements an UPDATE statement and return
** a pointer to that trigger step.  The parser calls this routine when it
** sees an UPDATE statement inside the body of a CREATE TRIGGER.
*/
 TriggerStep *sqlite3TriggerUpdateStep(
  sqlite3 *db,         /* The database connection */
  Token *pTableName,   /* Name of the table to be updated */
  ExprList *pEList,    /* The SET clause: list of column and new values */
  Expr *pWhere,        /* The WHERE clause */
  byte OnConflict            /* The conflict algorithm. (OE_Abort, OE_Ignore, etc) */
){
  TriggerStep *pTriggerStep;

  pTriggerStep = triggerStepAllocate(db, TK_UPDATE, pTableName);
  if( pTriggerStep ){
    pTriggerStep.ExprList = pEList.Dup();
    pTriggerStep.Where = pWhere.Dup();
    pTriggerStep.OnConflict = OnConflict;
  }
  db.ExprListDelete(pEList);
  db.ExprDelete(pWhere)
  return pTriggerStep;
}

/*
** Construct a trigger step that implements a DELETE statement and return
** a pointer to that trigger step.  The parser calls this routine when it
** sees a DELETE statement inside the body of a CREATE TRIGGER.
*/
 TriggerStep *sqlite3TriggerDeleteStep(
  sqlite3 *db,            /* Database connection */
  Token *pTableName,      /* The table from which rows are deleted */
  Expr *pWhere            /* The WHERE clause */
){
  TriggerStep *pTriggerStep;

  pTriggerStep = triggerStepAllocate(db, TK_DELETE, pTableName);
  if( pTriggerStep ){
    pTriggerStep.Where = pWhere.Dup();
    pTriggerStep.OnConflict = OE_Default;
  }
  db.ExprDelete(pWhere)
  return pTriggerStep;
}

//	Recursively delete a Trigger structure
func (db *sqlite3) DeleteTrigger(p *Trigger) {
	if p != nil {
		db.DeleteTriggerStep(p.Steps)
		p.Name = ""
		p.table = nil
		db.ExprDelete(p.pWhen)
		sqlite3IdListDelete(db, p.pColumns)
	}
}

/*
** This function is called to drop a trigger from the database schema. 
**
** This may be called directly from the parser and therefore identifies
** the trigger by name.  The sqlite3DropTriggerPtr() routine does the
** same job as this routine except it takes a pointer to the trigger
** instead of the trigger name.
**/
 void sqlite3DropTrigger(Parse *pParse, SrcList *pName, int noErr){
  Trigger *pTrigger = 0;
  int i;
  const char *zDb;
  const char *Name;
  int nName;
  sqlite3 *db = pParse.db;

  if( db.mallocFailed ) goto drop_trigger_cleanup;
  if pParse.ReadSchema() != SQLITE_OK {
	  goto drop_trigger_cleanup
  }

  assert( pName.nSrc==1 );
  zDb = pName.a[0].zDatabase;
  Name = pName.a[0].Name;
  nName = sqlite3Strlen30(Name);
  assert( zDb != 0 );
  for i, _ := range db.Databases {
    int j = (i<2) ? i^1 : i;  /* Search TEMP before MAIN */
    if !CaseInsensitiveMatch(db.Databases[j].Name, zDb) {
		continue
	}
    pTrigger = db.Databases[j].Schema.Triggers[Name]
    if( pTrigger ) break;
  }
  if( !pTrigger ){
    if( !noErr ){
      pParse.SetErrorMsg("no such trigger: %v", pName)
    }else{
      sqlite3CodeVerifyNamedSchema(pParse, zDb);
    }
    pParse.checkSchema = 1;
    goto drop_trigger_cleanup;
  }
  sqlite3DropTriggerPtr(pParse, pTrigger);

drop_trigger_cleanup:
  sqlite3SrcListDelete(db, pName);
}

//	Return a pointer to the Table structure for the table that a trigger is set on.
func (p *Trigger) table() *Table {
	return p.pTableSchema.Tables[p.table]
}

/*
** Drop a trigger given a pointer to that trigger. 
*/
 void sqlite3DropTriggerPtr(Parse *pParse, Trigger *pTrigger){
  Table   *pTable;
  Vdbe *v;
  sqlite3 *db = pParse.db;
  int iDb;

  iDb = pParse.db.SchemaToIndex(pTrigger.Schema)
  assert( iDb >= 0 && iDb < len(db.Databases) )
  pTable = tableOfTrigger(pTrigger);
  assert( pTable );
  assert( pTable.Schema==pTrigger.Schema || iDb==1 );
  {
    int code = SQLITE_DROP_TRIGGER;
    const char *zDb = db.Databases[iDb].Name;
    const char *zTab = SCHEMA_TABLE(iDb);
    if( iDb==1 ) code = SQLITE_DROP_TEMP_TRIGGER;
    if pParse.AuthCheck(code, pTrigger.Name, pTable.Name, zDb) != SQLITE_OK || pParse.AuthCheck(SQLITE_DELETE, zTab, 0, zDb) != SQLITE_OK {
      return;
    }
  }

	//	Generate code to destroy the database record of the trigger.
	assert( pTable != nil )
	if v = pParse.GetVdbe(); v != nil {
		pParse.BeginWriteOperation(0, iDb)
		pParse.OpenMasterTable(iDb)
		base := v.AddOpList(DROP_TRIGGER...)
		sqlite3VdbeChangeP4(v, base + 1, pTrigger.Name, P4_TRANSIENT)
		sqlite3VdbeChangeP4(v, base + 4, "trigger", P4_STATIC)
		sqlite3ChangeCookie(pParse, iDb);
		v.AddOp2(OP_Close, 0, 0)
		sqlite3VdbeAddOp4(v, OP_DropTrigger, iDb, 0, 0, pTrigger.Name, 0);
		if pParse.nMem < 3 {
			pParse.nMem = 3
		}
	}
}

//	Remove a trigger from the hash tables of the sqlite* pointer.
func (db *sqlite3) UnlinkAndDeleteTrigger(iDb int, Name string) {
	triggers := db.Databases[iDb].Schema.Triggers
	pTrigger := triggers[Name]
	delete(triggers, Name)
	if pTrigger != nil {
		if pTrigger.Schema == pTrigger.pTableSchema {
			Trigger **pp;
			for pp = pTrigger.table().pTrigger; pp != pTrigger; pp = pp.Next {}
			*pp = (*pp).Next
		}
		db.DeleteTrigger(pTrigger)
		db.flags |= SQLITE_InternChanges
	}
}

/*
** pEList is the SET clause of an UPDATE statement.  Each entry
** in pEList is of the format <id>=<expr>.  If any of the entries
** in pEList have an <id> which matches an identifier in IdList,
** then return TRUE.  If IdList==nil, then it is considered a
** wildcard that matches anything.  Likewise if pEList==NULL then
** it matches anything so always return true.  Return false only
** if there is no match.
*/
static int checkColumnOverlap(IdList *IdList, ExprList *pEList){
  int e;
  if( IdList==0 || pEList==0 ) return 1;
  for(e=0; e<pEList.nExpr; e++){
    if( sqlite3IdListIndex(IdList, pEList.a[e].Name)>=0 ) return 1;
  }
  return 0; 
}

/*
** Return a list of all triggers on table pTab if there exists at least
** one trigger that must be fired when an operation of type 'op' is 
** performed on the table, and, if that operation is an UPDATE, if at
** least one of the columns in pChanges is being modified.
*/
 Trigger *sqlite3TriggersExist(
  Parse *pParse,          /* Parse context */
  Table *pTab,            /* The table the contains the triggers */
  int op,                 /* one of TK_DELETE, TK_INSERT, TK_UPDATE */
  ExprList *pChanges,     /* Columns that change in an UPDATE statement */
  int *pMask              /* OUT: Mask of TRIGGER_BEFORE|TRIGGER_AFTER */
){
  int mask = 0;
  Trigger *pList = 0;
  Trigger *p;

  if( (pParse.db.flags & SQLITE_EnableTrigger)!=0 ){
    pList = pParse.TriggerList(pTab)
  }
  assert( pList==0 || pTab.IsVirtual()==0 );
  for(p=pList; p; p=p.Next){
    if( p.op==op && checkColumnOverlap(p.pColumns, pChanges) ){
      mask |= p.Time;
    }
  }
  if( pMask ){
    *pMask = mask;
  }
  return (mask ? pList : 0);
}

/*
** Convert the pStep.Target token into a SrcList and return a pointer
** to that SrcList.
**
** This routine adds a specific database name, if needed, to the target when
** forming the SrcList.  This prevents a trigger in one database from
** referring to a target in another database.  An exception is when the
** trigger is in TEMP in which case it can refer to any other database it
** wants.
*/
static SrcList *targetSrcList(
  Parse *pParse,       /* The parsing context */
  TriggerStep *pStep   /* The trigger containing the target token */
){
  int iDb;             /* Index of the database to use */
  SrcList *pSrc;       /* SrcList to be returned */

  pSrc = pParse.db.SrcListAppend(nil, &pStep.Target, "")
  if( pSrc ){
    assert( pSrc.nSrc>0 );
    assert( pSrc.a!=0 );
    iDb = pParse.db.SchemaToIndex(pStep.Trigger.Schema)
    if( iDb==0 || iDb>=2 ){
      db := pParse.db
      assert( iDb < len(db.Databases) )
      pSrc.a[pSrc.nSrc-1].zDatabase = sqlite3DbStrDup(db, db.Databases[iDb].Name);
    }
  }
  return pSrc;
}

/*
** Generate VDBE code for the statements inside the body of a single 
** trigger.
*/
static int codeTriggerProgram(
  Parse *pParse,            /* The parser context */
  TriggerStep *pStepList,   /* List of statements inside the trigger body */
  int OnConflict                /* Conflict algorithm. (OE_Abort, etc) */  
){
  TriggerStep *pStep;
  Vdbe *v = pParse.pVdbe;
  sqlite3 *db = pParse.db;

  assert( pParse.pTriggerTab && pParse.pToplevel );
  assert( pStepList );
  assert( v!=0 );
  for(pStep=pStepList; pStep; pStep=pStep.Next){
    /* Figure out the ON CONFLICT policy that will be used for this step
    ** of the trigger program. If the statement that caused this trigger
    ** to fire had an explicit ON CONFLICT, then use it. Otherwise, use
    ** the ON CONFLICT policy that was specified as part of the trigger
    ** step statement. Example:
    **
    **   CREATE TRIGGER AFTER INSERT ON t1 BEGIN;
    **     INSERT OR REPLACE INTO t2 VALUES(new.a, new.b);
    **   END;
    **
    **   INSERT INTO t1 ... ;            -- insert into t2 uses REPLACE policy
    **   INSERT OR IGNORE INTO t1 ... ;  -- insert into t2 uses IGNORE policy
    */
    pParse.eOrconf = (OnConflict==OE_Default)?pStep.OnConflict:(byte)OnConflict;

    switch( pStep.op ){
      case TK_UPDATE: {
        sqlite3Update(pParse, 
          targetSrcList(pParse, pStep),
          pStep.ExprList.Dup(), pStep.Where.Dup(), pParse.eOrconf);
        break;
      }
      case TK_INSERT: {
        sqlite3Insert(pParse, 
          targetSrcList(pParse, pStep),
          pStep.ExprList.Dup(), 
          pStep.Select.Dup(), 
          pStep.IdList.Dup(), 
          pParse.eOrconf
        );
        break;
      }
      case TK_DELETE: {
        sqlite3DeleteFrom(pParse, 
          targetSrcList(pParse, pStep), pStep.Where.Dup());
        break;
      }
      default: assert( pStep.op==TK_SELECT ); {
        SelectDest sDest;
        Select *pSelect = pStep.Select.Dup();
        sqlite3SelectDestInit(&sDest, SRT_Discard, 0);
        sqlite3Select(pParse, pSelect, &sDest);
        sqlite3SelectDelete(db, pSelect);
        break;
      }
    } 
    if( pStep.op!=TK_SELECT ){
      v.AddOp0(OP_ResetCount);
    }
  }

  return 0;
}

/*
** Parse context structure pFrom has just been used to create a sub-vdbe
** (trigger program). If an error has occurred, transfer error information
** from pFrom to pTo.
*/
static void transferParseError(Parse *pTo, Parse *pFrom){
  assert( pFrom.zErrMsg==0 || pFrom.nErr );
  assert( pTo.zErrMsg==0 || pTo.nErr );
  if( pTo.nErr==0 ){
    pTo.zErrMsg = pFrom.zErrMsg;
    pTo.nErr = pFrom.nErr;
  }else{
    pFrom.zErrMsg = nil
  }
}

/*
** Create and populate a new TriggerPrg object with a sub-program 
** implementing trigger pTrigger with ON CONFLICT policy OnConflict.
*/
static TriggerPrg *codeRowTrigger(
  Parse *pParse,       /* Current parse context */
  Trigger *pTrigger,   /* Trigger to code */
  Table *pTab,         /* The table pTrigger is attached to */
  int OnConflict           /* ON CONFLICT policy to code trigger program with */
){
	pTop := pParse.Toplevel()
  sqlite3 *db = pParse.db;   /* Database handle */
  TriggerPrg *pPrg;           /* Value to return */
  Expr *pWhen = 0;            /* Duplicate of trigger WHEN expression */
  Vdbe *v;                    /* Temporary VM */
  NameContext sNC;            /* Name context for sub-vdbe */
  Parse *pSubParse;           /* Parse context for sub-vdbe */
  int iEndTrigger = 0;        /* Label to jump to if WHEN is false */

	routine		*SubProgram
  assert( pTrigger.Name == "" || pTab == pTrigger.table() )
  assert( pTop.pVdbe );

  /* Allocate the TriggerPrg and SubProgram objects. To ensure that they
  ** are freed if an error occurs, link them into the Parse.pTriggerPrg 
  ** list of the top-level Parse object sooner rather than later.  */
  pPrg = sqlite3DbMallocZero(db, sizeof(TriggerPrg));
  if( !pPrg ) return 0;
  pPrg.Next = pTop.pTriggerPrg;
  pTop.pTriggerPrg = pPrg;
  routine = sqlite3DbMallocZero(db, sizeof(SubProgram))
  pPrg.SubProgram = routine
  if routine == nil {
	  return nil
  }
  pTop.pVdbe.LinkRoutine(routine)
  pPrg.pTrigger = pTrigger;
  pPrg.OnConflict = OnConflict;
  pPrg.Columnsmask[0] = 0xffffffff;
  pPrg.Columnsmask[1] = 0xffffffff;

  /* Allocate and populate a new Parse context to use for coding the 
  ** trigger sub-program.  */
  pSubParse = sqlite3StackAllocZero(db, sizeof(Parse));
  if( !pSubParse ) return 0;
  memset(&sNC, 0, sizeof(sNC));
  sNC.Parse = pSubParse;
  pSubParse.db = db;
  pSubParse.pTriggerTab = pTab;
  pSubParse.pToplevel = pTop;
  pSubParse.zAuthContext = pTrigger.Name;
  pSubParse.eTriggerOp = pTrigger.op;
  pSubParse.nQueryLoop = pParse.nQueryLoop;

  v = pSubParse.GetVdbe()
  if( v ){
    v.Comment("Start: ", pTrigger.Name, ".", onErrorText(OnConflict), " (%v %v%v%v ON %v)", 
		(pTrigger.Time==TRIGGER_BEFORE ? "BEFORE" : "AFTER"),
        (pTrigger.op==TK_UPDATE ? "UPDATE" : ""),
        (pTrigger.op==TK_INSERT ? "INSERT" : ""),
        (pTrigger.op==TK_DELETE ? "DELETE" : ""),
      pTab.Name
    )
#ifndef SQLITE_OMIT_TRACE
    sqlite3VdbeChangeP4(v, -1, fmt.Sprintf("-- TRIGGER %v", pTrigger.Name), P4_DYNAMIC);
#endif

    /* If one was specified, code the WHEN clause. If it evaluates to false
    ** (or NULL) the sub-vdbe is immediately halted by jumping to the 
    ** OP_Halt inserted at the end of the program.  */
    if( pTrigger.pWhen ){
      pWhen = pTrigger.pWhen.Dup();
      if( SQLITE_OK==sqlite3ResolveExprNames(&sNC, pWhen) 
       && !db.mallocFailed
      ){
        iEndTrigger = v.MakeLabel()
        sqlite3ExprIfFalse(pSubParse, pWhen, iEndTrigger, SQLITE_JUMPIFNULL);
      }
      db.ExprDelete(pWhen)
    }

    /* Code the trigger program into the sub-vdbe. */
    codeTriggerProgram(pSubParse, pTrigger.Steps, OnConflict);

    /* Insert an OP_Halt at the end of the sub-program. */
    if( iEndTrigger ){
      v.ResolveLabel(iEndTrigger)
    }
    v.AddOp0(OP_Halt);
    v.Comment("End: ", pTrigger.Name, ".", onErrorText(OnConflict))

    transferParseError(pParse, pSubParse);
    if !db.mallocFailed {
      routine.Opcodes, pTop.nMaxArg = v.TakeOpArray(pTop.nMaxArg)
    }
    routine.nMem = pSubParse.nMem;
    routine.nCsr = pSubParse.nTab;
    routine.nOnce = pSubParse.nOnce;
    routine.token = (void *)pTrigger;
    pPrg.Columnsmask[0] = pSubParse.oldmask;
    pPrg.Columnsmask[1] = pSubParse.newmask;
    sqlite3VdbeDelete(v);
  }

  assert( !pSubParse.pAinc       && !pSubParse.pZombieTab );
  assert( !pSubParse.pTriggerPrg && !pSubParse.nMaxArg );
  sqlite3StackFree(db, pSubParse);

  return pPrg;
}
    
/*
** Return a pointer to a TriggerPrg object containing the sub-program for
** trigger pTrigger with default ON CONFLICT algorithm OnConflict. If no such
** TriggerPrg object exists, a new object is allocated and populated before
** being returned.
*/
static TriggerPrg *getRowTrigger(
  Parse *pParse,       /* Current parse context */
  Trigger *pTrigger,   /* Trigger to code */
  Table *pTab,         /* The table trigger pTrigger is attached to */
  int OnConflict           /* ON CONFLICT algorithm. */
){
	pRoot := pParse.Toplevel()
  TriggerPrg *pPrg;

  assert( pTrigger.Name == "" || pTab == pTrigger.table() )

  /* It may be that this trigger has already been coded (or is in the
  ** process of being coded). If this is the case, then an entry with
  ** a matching TriggerPrg.pTrigger field will be present somewhere
  ** in the Parse.pTriggerPrg list. Search for such an entry.  */
  for(pPrg=pRoot.pTriggerPrg; 
      pPrg && (pPrg.pTrigger!=pTrigger || pPrg.OnConflict!=OnConflict); 
      pPrg=pPrg.Next
  );

  /* If an existing TriggerPrg could not be located, create a new one. */
  if( !pPrg ){
    pPrg = codeRowTrigger(pParse, pTrigger, pTab, OnConflict);
  }

  return pPrg;
}

/*
** Generate code for the trigger program associated with trigger p on 
** table pTab. The reg, OnConflict and ignoreJump parameters passed to this
** function are the same as those described in the header function for
** sqlite3CodeRowTrigger()
*/
void sqlite3CodeRowTriggerDirect(
  Parse *pParse,       /* Parse context */
  Trigger *p,          /* Trigger to code */
  Table *pTab,         /* The table to code triggers from */
  int reg,             /* Reg array containing OLD.* and NEW.* values */
  int OnConflict,          /* ON CONFLICT policy */
  int ignoreJump       /* Instruction to jump to for RAISE(IGNORE) */
){
	v = pParse.GetVdbe() /* Main VM */
  TriggerPrg *pPrg;
  pPrg = getRowTrigger(pParse, p, pTab, OnConflict);
  assert( pPrg || pParse.nErr || pParse.db.mallocFailed );

  /* Code the OP_Program opcode in the parent VDBE. P4 of the OP_Program 
  ** is a pointer to the sub-vdbe containing the trigger program.  */
  if( pPrg ){
    int bRecursive = (p.Name && 0==(pParse.db.flags&SQLITE_RecTriggers));

    v.AddOp3(OP_Program, reg, ignoreJump, ++pParse.nMem);
    sqlite3VdbeChangeP4(v, -1, (const char *)pPrg.SubProgram, P4_SUBPROGRAM);
	if p.Name != nil {
		v.Comment("Call: ", pName, ".", onErrorText(OnConflict))
	} else {
		v.Comment("Call: fkey.", onErrorText(OnConflict))
	}

    /* Set the P5 operand of the OP_Program instruction to non-zero if
    ** recursive invocation of this trigger program is disallowed. Recursive
    ** invocation is disallowed if (a) the sub-program is really a trigger,
    ** not a foreign key action, and (b) the flag to enable recursive triggers
    ** is clear.  */
    v.ChangeP5(byte(bRecursive))
  }
}

/*
** This is called to code the required FOR EACH ROW triggers for an operation
** on table pTab. The operation to code triggers for (INSERT, UPDATE or DELETE)
** is given by the op paramater. The Time parameter determines whether the
** BEFORE or AFTER triggers are coded. If the operation is an UPDATE, then
** parameter pChanges is passed the list of columns being modified.
**
** If there are no triggers that fire at the specified time for the specified
** operation on pTab, this function is a no-op.
**
** The reg argument is the address of the first in an array of registers 
** that contain the values substituted for the new.* and old.* references
** in the trigger program. If N is the number of columns in table pTab
** (a copy of pTab.nCol), then registers are populated as follows:
**
**   Register       Contains
**   ------------------------------------------------------
**   reg+0          OLD.rowid
**   reg+1          OLD.* value of left-most column of pTab
**   ...            ...
**   reg+N          OLD.* value of right-most column of pTab
**   reg+N+1        NEW.rowid
**   reg+N+2        OLD.* value of left-most column of pTab
**   ...            ...
**   reg+N+N+1      NEW.* value of right-most column of pTab
**
** For ON DELETE triggers, the registers containing the NEW.* values will
** never be accessed by the trigger program, so they are not allocated or 
** populated by the caller (there is no data to populate them with anyway). 
** Similarly, for ON INSERT triggers the values stored in the OLD.* registers
** are never accessed, and so are not allocated by the caller. So, for an
** ON INSERT trigger, the value passed to this function as parameter reg
** is not a readable register, although registers (reg+N) through 
** (reg+N+N+1) are.
**
** Parameter OnConflict is the default conflict resolution algorithm for the
** trigger program to use (REPLACE, IGNORE etc.). Parameter ignoreJump
** is the instruction that control should jump to if a trigger program
** raises an IGNORE exception.
*/
 void sqlite3CodeRowTrigger(
  Parse *pParse,       /* Parse context */
  Trigger *pTrigger,   /* List of triggers on table pTab */
  int op,              /* One of TK_UPDATE, TK_INSERT, TK_DELETE */
  ExprList *pChanges,  /* Changes list for any UPDATE OF triggers */
  int Time,           /* One of TRIGGER_BEFORE, TRIGGER_AFTER */
  Table *pTab,         /* The table to code triggers from */
  int reg,             /* The first in an array of registers (see above) */
  int OnConflict,          /* ON CONFLICT policy */
  int ignoreJump       /* Instruction to jump to for RAISE(IGNORE) */
){
  Trigger *p;          /* Used to iterate through pTrigger list */

  assert( op==TK_UPDATE || op==TK_INSERT || op==TK_DELETE );
  assert( Time==TRIGGER_BEFORE || Time==TRIGGER_AFTER );
  assert( (op==TK_UPDATE)==(pChanges!=0) );

  for(p=pTrigger; p; p=p.Next){

    /* Sanity checking:  The schema for the trigger and for the table are
    ** always defined.  The trigger must be in the same schema as the table
    ** or else it must be a TEMP trigger. */
    assert( p.Schema!=0 );
    assert( p.pTableSchema!=0 );
    assert( p.Schema==p.pTableSchema 
         || p.Schema==pParse.db.Databases[1].Schema );

    /* Determine whether we should code this trigger */
    if( p.op==op 
     && p.Time==Time 
     && checkColumnOverlap(p.pColumns, pChanges)
    ){
      sqlite3CodeRowTriggerDirect(pParse, p, pTab, reg, OnConflict, ignoreJump);
    }
  }
}

/*
** Triggers may access values stored in the old.* or new.* pseudo-table. 
** This function returns a 32-bit bitmask indicating which columns of the 
** old.* or new.* tables actually are used by triggers. This information 
** may be used by the caller, for example, to avoid having to load the entire
** old.* record into memory when executing an UPDATE or DELETE command.
**
** Bit 0 of the returned mask is set if the left-most column of the
** table may be accessed using an [old|new].<col> reference. Bit 1 is set if
** the second leftmost column value is required, and so on. If there
** are more than 32 columns in the table, and at least one of the columns
** with an index greater than 32 may be accessed, 0xffffffff is returned.
**
** It is not possible to determine if the old.rowid or new.rowid column is 
** accessed by triggers. The caller must always assume that it is.
**
** Parameter isNew must be either 1 or 0. If it is 0, then the mask returned
** applies to the old.* table. If 1, the new.* table.
**
** Parameter Time must be a mask with one or both of the TRIGGER_BEFORE
** and TRIGGER_AFTER bits set. Values accessed by BEFORE triggers are only
** included in the returned mask if the TRIGGER_BEFORE bit is set in the
** Time parameter. Similarly, values accessed by AFTER triggers are only
** included in the returned mask if the TRIGGER_AFTER bit is set in Time.
*/
 uint32 sqlite3TriggerColmask(
  Parse *pParse,       /* Parse context */
  Trigger *pTrigger,   /* List of triggers on table pTab */
  ExprList *pChanges,  /* Changes list for any UPDATE OF triggers */
  int isNew,           /* 1 for new.* ref mask, 0 for old.* ref mask */
  int Time,           /* Mask of TRIGGER_BEFORE|TRIGGER_AFTER */
  Table *pTab,         /* The table to code triggers from */
  int OnConflict           /* Default ON CONFLICT policy for trigger steps */
){
  const int op = pChanges ? TK_UPDATE : TK_DELETE;
  uint32 mask = 0;
  Trigger *p;

  assert( isNew==1 || isNew==0 );
  for(p=pTrigger; p; p=p.Next){
    if( p.op==op && (Time&p.Time)
     && checkColumnOverlap(p.pColumns,pChanges)
    ){
      TriggerPrg *pPrg;
      pPrg = getRowTrigger(pParse, p, pTab, OnConflict);
      if( pPrg ){
        mask |= pPrg.Columnsmask[isNew];
      }
    }
  }

  return mask;
}