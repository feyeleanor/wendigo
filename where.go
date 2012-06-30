/*
** This module contains C code that generates VDBE code used to process
** the WHERE clause of SQL statements.  This module is responsible for
** generating the code that loops through a table looking for applicable
** rows.  Indices are selected and used to speed the search when doing
** so is applicable.  Because this module is responsible for selecting
** indices, you might also think of this module as the "query optimizer".
*/

# define WHERETRACE(X)

typedef struct WhereMaskSet WhereMaskSet;
typedef struct WhereOrInfo WhereOrInfo;
typedef struct WhereAndInfo WhereAndInfo;
typedef struct WhereCost WhereCost;

//	The query generator uses an array of instances of this structure to help it analyze the subexpressions of the WHERE clause. Each WHERE clause subexpression is separated from the others by AND operators, usually, or sometimes subexpressions separated by OR.
//	All WhereTerms are collected into a single WhereClause structure. The following identity holds:
//			WhereTerm.pWC.Terms[WhereTerm.idx] == WhereTerm
//	When a term is of the form:
//			X <op> <expr>
//	where X is a column name and <op> is one of certain operators, then WhereTerm.leftCursor and WhereTerm.u.leftColumn record the cursor number and column number for X. WhereTerm.eOperator records the <op> using a bitmask encoding defined by WO_xxx below. The use of a bitmask encoding for the operator allows us to search quickly for terms that match any of several different operators.
//	A WhereTerm might also be two or more subterms connected by OR:
//			(t1.X <op> <expr>) OR (t1.Y <op> <expr>) OR ....
//	In this second case, wtFlag as the TERM_ORINFO set and eOperator == WO_OR and the WhereTerm.u.pOrInfo field points to auxiliary information that is collected about the
//	If a term in the WHERE clause does not match either of the two previous categories, then eOperator == 0. The WhereTerm.Expr field is still set to the original subexpression content and wtFlags is set up appropriately but no other fields in the WhereTerm object are meaningful.
//	When eOperator != 0, prereqRight and prereqAll record sets of cursor numbers, but they do so indirectly. A single WhereMaskSet structure translates cursor number into bits and the translated bit is stored in the prereq fields. The translation is used in order to maximize the number of bits that will fit in a Bitmask. The VDBE cursor numbers might be spread out over the non-negative integers. For example, the cursor numbers might be 3, 8, 9, 10, 20, 23, 41, and 45. The WhereMaskSet translates these sparse cursor numbers into consecutive integers beginning with 0 in order to make the best possible use of the available bits in the Bitmask. So, in the example above, the cursor numbers would be mapped into integers 0 through 7.
//	The number of terms in a join is limited by the number of bits in prereqRight and prereqAll. The default is 64 bits, hence SQLite is only able to process joins with 64 or fewer tables.
typedef struct WhereTerm WhereTerm;
struct WhereTerm {
  Expr *pExpr;            /* Pointer to the subexpression that is this term */
  int iParent;            /* Disable pWC.Terms[iParent] when this term disabled */
  int leftCursor;         /* Cursor number of X in "X <op> <expr>" */
  union {
    int leftColumn;         /* Column number of X in "X <op> <expr>" */
    WhereOrInfo *pOrInfo;   /* Extra information if eOperator==WO_OR */
    WhereAndInfo *pAndInfo; /* Extra information if eOperator==WO_AND */
  } u;
  uint16 eOperator;          /* A WO_xx value describing <op> */
  byte wtFlags;             /* TERM_xxx bit flags.  See below */
  byte nChild;              /* Number of children that must disable us */
  WhereClause *pWC;       /* The clause this term is part of */
  Bitmask prereqRight;    /* Bitmask of tables used by pExpr.pRight */
  Bitmask prereqAll;      /* Bitmask of tables referenced by pExpr */
};

//	Allowed values of WhereTerm.wtFlags
#define TERM_DYNAMIC    0x01   /* Need to call db.ExprDelete(pExpr) */
#define TERM_VIRTUAL    0x02   /* Added by the optimizer.  Do not code */
#define TERM_CODED      0x04   /* This term is already coded */
#define TERM_COPIED     0x08   /* Has a child */
#define TERM_ORINFO     0x10   /* Need to free the WhereTerm.u.pOrInfo object */
#define TERM_ANDINFO    0x20   /* Need to free the WhereTerm.u.pAndInfo obj */
#define TERM_OR_OK      0x40   /* Used during OR-clause processing */
#define TERM_VNULL		0x80   /* Manufactured x>NULL or x<=NULL term */

//	A WhereTerm with eOperator==WO_OR has its u.pOrInfo pointer set to a dynamically allocated instance of the following structure.
struct WhereOrInfo {
  WhereClause wc;          /* Decomposition into subterms */
  Bitmask indexable;       /* Bitmask of all indexable tables in the clause */
};

//	A WhereTerm with eOperator==WO_AND has its u.pAndInfo pointer set to a dynamically allocated instance of the following structure.
struct WhereAndInfo {
  WhereClause wc;          /* The subexpression broken out */
};

//	An instance of the following structure keeps track of a mapping between VDBE cursor numbers and bits of the bitmasks in WhereTerm.
//	The VDBE cursor numbers are small integers contained in  SrcList_item.iCursor and Expr.iTable fields. For any given WHERE clause, the cursor numbers might not begin with 0 and they might contain gaps in the numbering sequence. But we want to make maximum use of the bits in our bitmasks. This structure provides a mapping from the sparse cursor numbers into consecutive integers beginning with 0.
//	If WhereMaskSet.ix[A] == B it means that The A-th bit of a Bitmask corresponds VDBE cursor number B. The A-th bit of a bitmask is 1 << A.
//	For example, if the WHERE clause expression used these VDBE cursors: 4, 5, 8, 29, 57, 73. Then the  WhereMaskSet structure would map those cursor numbers into bits 0 through 5.
//	Note that the mapping is not necessarily ordered. In the example above, the mapping might go like this: 4.3, 5.1, 8.2, 29.0, 57.5, 73.4. Or one of 719 other combinations might be used. It does not really matter. What is important is that sparse cursor numbers all get mapped into bit numbers that begin with 0 and contain no gaps.
struct WhereMaskSet {
  int n;                        /* Number of assigned cursor values */
  int ix[BMS];                  /* Cursor assigned to each bit */
};

//	A WhereCost object records a lookup strategy and the estimated cost of pursuing that strategy.
struct WhereCost {
  WherePlan plan;    /* The lookup strategy */
  double rCost;      /* Overall cost of pursuing this search strategy */
  Bitmask used;      /* Bitmask of cursors used by this plan */
};

//	Bitmasks for the operators that indices are able to exploit. An OR-ed combination of these values can be used when searching for terms in the where clause.
#define WO_IN     0x001
#define WO_EQ     0x002
#define WO_LT     (WO_EQ<<(TK_LT-TK_EQ))
#define WO_LE     (WO_EQ<<(TK_LE-TK_EQ))
#define WO_GT     (WO_EQ<<(TK_GT-TK_EQ))
#define WO_GE     (WO_EQ<<(TK_GE-TK_EQ))
#define WO_MATCH  0x040
#define WO_ISNULL 0x080
#define WO_OR     0x100       /* Two or more OR-connected terms */
#define WO_AND    0x200       /* Two or more AND-connected terms */
#define WO_NOOP   0x800       /* This term does not restrict search space */

#define WO_ALL    0xfff       /* Mask of all possible WO_* values */
#define WO_SINGLE 0x0ff       /* Mask of all non-compound WO_* values */

//	Value for wsFlags returned by bestIndex() and stored in WhereLevel.wsFlags. These flags determine which search strategies are appropriate.
//	The least significant 12 bits is reserved as a mask for WO_ values above. The WhereLevel.wsFlags field is usually set to WO_IN | WO_EQ | WO_ISNULL. But if the table is the right table of a left join, WhereLevel.wsFlags is set to WO_IN | WO_EQ. The WhereLevel.wsFlags field can then be used as the "op" parameter to findTerm when we are resolving equality constraints. ISNULL constraints will then not be used on the right table of a left join.
#define WHERE_ROWID_EQ     0x00001000  /* rowid=EXPR or rowid IN (...) */
#define WHERE_ROWID_RANGE  0x00002000  /* rowid<EXPR and/or rowid>EXPR */
#define WHERE_COLUMN_EQ    0x00010000  /* x=EXPR or x IN (...) or x IS NULL */
#define WHERE_COLUMN_RANGE 0x00020000  /* x<EXPR and/or x>EXPR */
#define WHERE_COLUMN_IN    0x00040000  /* x IN (...) */
#define WHERE_COLUMN_NULL  0x00080000  /* x IS NULL */
#define WHERE_INDEXED      0x000f0000  /* Anything that uses an index */
#define WHERE_NOT_FULLSCAN 0x100f3000  /* Does not do a full table scan */
#define WHERE_IN_ABLE      0x000f1000  /* Able to support an IN operator */
#define WHERE_TOP_LIMIT    0x00100000  /* x<EXPR or x<=EXPR constraint */
#define WHERE_BTM_LIMIT    0x00200000  /* x>EXPR or x>=EXPR constraint */
#define WHERE_BOTH_LIMIT   0x00300000  /* Both x>EXPR and x<EXPR */
#define WHERE_IDX_ONLY     0x00800000  /* Use index only - omit table */
#define WHERE_ORDERBY      0x01000000  /* Output will appear in correct order */
#define WHERE_REVERSE      0x02000000  /* Scan in reverse order */
#define WHERE_UNIQUE       0x04000000  /* Selects no more than one row */
#define WHERE_VIRTUALTABLE 0x08000000  /* Use virtual-table processing */
#define WHERE_MULTI_OR     0x10000000  /* OR using multiple indices */
#define WHERE_TEMP_INDEX   0x20000000  /* Uses an ephemeral index */
#define WHERE_DISTINCT     0x40000000  /* Correct order for DISTINCT */


//	Deallocate all memory associated with a WhereOrInfo object.
static void whereOrInfoDelete(sqlite3 *db, WhereOrInfo *p){
  p.wc.Clear()
  p = nil
}

//	Deallocate all memory associated with a WhereAndInfo object.
static void whereAndInfoDelete(sqlite3 *db, WhereAndInfo *p){
  p.wc.Clear()
  p = nil
}

//	Return the bitmask for the given cursor number.  Return 0 if iCursor is not in the set.
static Bitmask getMask(WhereMaskSet *pMaskSet, int iCursor){
  int i;
  assert( pMaskSet.n<=(int)sizeof(Bitmask)*8 );
  for(i=0; i<pMaskSet.n; i++){
    if( pMaskSet.ix[i]==iCursor ){
      return ((Bitmask)1)<<i;
    }
  }
  return 0;
}

//	Create a new mask for cursor iCursor.
//	There is one cursor per table in the FROM clause. The number of tables in the FROM clause is limited by a test early in the WhereBegin() routine. So we know that the pMaskSet.ix[] array will never overflow.
static void createMask(WhereMaskSet *pMaskSet, int iCursor){
  assert( pMaskSet.n < ArraySize(pMaskSet.ix) );
  pMaskSet.ix[pMaskSet.n++] = iCursor;
}

/*
** This routine walks (recursively) an expression tree and generates
** a bitmask indicating which tables are used in that expression
** tree.
**
** In order for this routine to work, the calling function must have
** previously invoked sqlite3ResolveExprNames() on the expression.  See
** the header comment on that routine for additional information.
** The sqlite3ResolveExprNames() routines looks for column names and
** sets their opcodes to TK_COLUMN and their Expr.iTable fields to
** the VDBE cursor number of the table.  This routine just has to
** translate the cursor numbers into bitmask values and OR all
** the bitmasks together.
*/
static Bitmask exprListTableUsage(WhereMaskSet*, ExprList*);
static Bitmask exprSelectTableUsage(WhereMaskSet*, Select*);
static Bitmask exprTableUsage(WhereMaskSet *pMaskSet, Expr *p){
  Bitmask mask = 0;
  if( p==0 ) return 0;
  if( p.op==TK_COLUMN ){
    mask = getMask(pMaskSet, p.iTable);
    return mask;
  }
  mask = exprTableUsage(pMaskSet, p.pRight);
  mask |= exprTableUsage(pMaskSet, p.pLeft);
  if( p.HasProperty(EP_xIsSelect) ){
    mask |= exprSelectTableUsage(pMaskSet, p.x.Select);
  }else{
    mask |= exprListTableUsage(pMaskSet, p.pList);
  }
  return mask;
}
static Bitmask exprListTableUsage(WhereMaskSet *pMaskSet, ExprList *pList){
  int i;
  Bitmask mask = 0;
  if( pList ){
    for(i=0; i<pList.nExpr; i++){
      mask |= exprTableUsage(pMaskSet, pList.a[i].Expr);
    }
  }
  return mask;
}

static Bitmask exprSelectTableUsage(WhereMaskSet *pMaskSet, Select *pS){
	Bitmask mask = 0
	for pS != nil {
		mask |= exprListTableUsage(pMaskSet, pS.pEList)
		mask |= exprListTableUsage(pMaskSet, pS.pGroupBy)
		mask |= exprListTableUsage(pMaskSet, pS.pOrderBy)
		mask |= exprTableUsage(pMaskSet, pS.Where)
		mask |= exprTableUsage(pMaskSet, pS.pHaving)
		if pSrc := pS.pSrc; pSrc != 0 {
			for i := 0; i < len(pSrc); i++ {
				mask |= exprSelectTableUsage(pMaskSet, pSrc[i].Select)
				mask |= exprTableUsage(pMaskSet, pSrc[i].pOn)
			}
		}
		pS = pS.pPrior
	}
	return mask
}

//	Return TRUE if the given operator is one of the operators that is allowed for an indexable WHERE clause term. The allowed operators are "=", "<", ">", "<=", ">=", and "IN".
//	IMPLEMENTATION-OF: R-59926-26393 To be usable by an index a term must be of one of the following forms: column = expression column > expression column >= expression column < expression column <= expression expression = column expression > column expression >= column expression < column expression <= column column IN (expression-list) column IN (subquery) column IS NULL
static int allowedOp(int op){
	return op == TK_IN || (op >= TK_EQ && op <= TK_GE) || op == TK_ISNULL
}

/*
** Commute a comparison operator.  Expressions of the form "X op Y"
** are converted into "Y op X".
**
** If a collation sequence is associated with either the left or right
** side of the comparison, it remains associated with the same side after
** the commutation. So "Y collate NOCASE op X" becomes 
** "X collate NOCASE op Y". This is because any collation sequence on
** the left hand side of a comparison overrides any collation sequence 
** attached to the right. For the same reason the EP_ExpCollate flag
** is not commuted.
*/
static void exprCommute(Parse *pParse, Expr *pExpr){
  uint16 expRight = (pExpr.pRight.flags & EP_ExpCollate);
  uint16 expLeft = (pExpr.pLeft.flags & EP_ExpCollate);
  assert( allowedOp(pExpr.op) && pExpr.op!=TK_IN );
  pExpr.pRight.pColl = sqlite3ExprCollSeq(pParse, pExpr.pRight);
  pExpr.pLeft.pColl = sqlite3ExprCollSeq(pParse, pExpr.pLeft);
  pExpr.pRight.pColl, pExpr.pLeft.pColl = pExpr.pLeft.pColl, pExpr.pRight.pColl
  pExpr.pRight.flags = (pExpr.pRight.flags & ~EP_ExpCollate) | expLeft;
  pExpr.pLeft.flags = (pExpr.pLeft.flags & ~EP_ExpCollate) | expRight;
  pExpr.pRight, pExpr.pLeft = pExpr.pLeft, pExpr.pRight
  if( pExpr.op>=TK_GT ){
    assert( TK_LT==TK_GT+2 );
    assert( TK_GE==TK_LE+2 );
    assert( TK_GT>TK_EQ );
    assert( TK_GT<TK_LE );
    assert( pExpr.op>=TK_GT && pExpr.op<=TK_GE );
    pExpr.op = ((pExpr.op-TK_GT)^2)+TK_GT;
  }
}

/*
** Translate from TK_xx operator to WO_xx bitmask.
*/
static uint16 operatorMask(int op){
  uint16 c;
  assert( allowedOp(op) );
  if( op==TK_IN ){
    c = WO_IN;
  }else if( op==TK_ISNULL ){
    c = WO_ISNULL;
  }else{
    assert( (WO_EQ<<(op-TK_EQ)) < 0x7fff );
    c = (uint16)(WO_EQ<<(op-TK_EQ));
  }
  assert( op!=TK_ISNULL || c==WO_ISNULL );
  assert( op!=TK_IN || c==WO_IN );
  assert( op!=TK_EQ || c==WO_EQ );
  assert( op!=TK_LT || c==WO_LT );
  assert( op!=TK_LE || c==WO_LE );
  assert( op!=TK_GT || c==WO_GT );
  assert( op!=TK_GE || c==WO_GE );
  return c;
}

/*
** Search for a term in the WHERE clause that is of the form "X <op> <expr>"
** where X is a reference to the iColumn of table iCur and <op> is one of
** the WO_xx operator codes specified by the op parameter.
** Return a pointer to the term.  Return 0 if not found.
*/
static WhereTerm *findTerm(
  WhereClause *pWC,     /* The WHERE clause to be searched */
  int iCur,             /* Cursor number of LHS */
  int iColumn,          /* Column number of LHS */
  Bitmask notReady,     /* RHS must not overlap with this mask */
  uint32 op,               /* Mask of WO_xx values describing operator */
  Index *pIdx           /* Must be compatible with this index, if not NULL */
){
  WhereTerm *pTerm;
  int k;
  assert( iCur>=0 );
  op &= WO_ALL;
  for ; pWC != nil; pWC = pWC.OuterConjunction {
    for(pTerm=pWC.Terms, k=len(pWC.Terms); k; k--, pTerm++){
      if( pTerm.leftCursor==iCur
         && (pTerm.prereqRight & notReady)==0
         && pTerm.u.leftColumn==iColumn
         && (pTerm.eOperator & op)!=0
      ){
        if( iColumn>=0 && pIdx && pTerm.eOperator!=WO_ISNULL ){
          Expr *pX = pTerm.Expr;
          CollSeq *pColl;
          char idxaff;
          int j;
          Parse *pParse = pWC.Parse;
  
          idxaff = pIdx.pTable.Columns[iColumn].affinity;
          if( !sqlite3IndexAffinityOk(pX, idxaff) ) continue;
  
          /* Figure out the collation sequence required from an index for
          ** it to be useful for optimising expression pX. Store this
          ** value in variable pColl.
          */
          assert(pX.pLeft);
          pColl = sqlite3BinaryCompareCollSeq(pParse, pX.pLeft, pX.pRight);
          assert(pColl || pParse.nErr);
  
          for j := 0; pIdx.Columns[j] != iColumn; j++ {
			  if j >= len(pIdx.Columns) {
				  return 0
			  }
		  }
          if pColl != nil && !CaseInsensitiveMatch(pColl.Name, pIdx.azColl[j]) {
			  continue
			}
        }
        return pTerm;
      }
    }
  }
  return 0;
}

//	Call exprAnalyze on all terms in a WHERE clause.  
static void exprAnalyzeAll(
  SrcList *pTabList,       /* the FROM clause */
  WhereClause *pWC         /* the WHERE clause to be analyzed */
){
  int i;
  for(i=len(pWC.Terms)-1; i>=0; i--){
    exprAnalyze(pTabList, pWC, i);
  }
}

#ifndef SQLITE_OMIT_LIKE_OPTIMIZATION
/*
** Check to see if the given expression is a LIKE or GLOB operator that
** can be optimized using inequality constraints.  Return TRUE if it is
** so and false if not.
**
** In order for the operator to be optimizible, the RHS must be a string
** literal that does not begin with a wildcard.  
*/
static int isLikeOrGlob(
  Parse *pParse,    /* Parsing and code generating context */
  Expr *pExpr,      /* Test this expression */
  Expr **ppPrefix,  /* Pointer to TK_STRING expression with pattern prefix */
  int *pisComplete, /* True if the only wildcard is % in the last character */
  int *pnoCase      /* True if uppercase is equivalent to lowercase */
){
  const char *z = 0;         /* String on RHS of LIKE operator */
  Expr *pRight, *pLeft;      /* Right and left size of LIKE operator */
  ExprList *pList;           /* List of operands to the LIKE operator */
  int c;                     /* One character in z[] */
  int cnt;                   /* Number of non-wildcard prefix characters */
  char wc[3];                /* Wildcard characters */
  sqlite3 *db = pParse.db;  /* Database connection */
  sqlite3_value *pVal = 0;
  int op;                    /* Opcode of pRight */

  if( !sqlite3IsLikeFunction(db, pExpr, pnoCase, wc) ){
    return 0;
  }
  pList = pExpr.pList;
  pLeft = pList.a[1].Expr;
  if( pLeft.op!=TK_COLUMN 
   || sqlite3ExprAffinity(pLeft)!=SQLITE_AFF_TEXT 
   || pLeft.pTab.IsVirtual()
  ){
    /* IMP: R-02065-49465 The left-hand side of the LIKE or GLOB operator must
    ** be the name of an indexed column with TEXT affinity. */
    return 0;
  }
  assert( pLeft.iColumn!=(-1) ); /* Because IPK never has AFF_TEXT */

  pRight = pList.a[0].Expr;
  op = pRight.op;
  if( op==TK_REGISTER ){
    op = pRight.op2;
  }
  if( op==TK_VARIABLE ){
    Vdbe *pReprepare = pParse.pReprepare;
    int iCol = pRight.iColumn;
    pVal = sqlite3VdbeGetValue(pReprepare, iCol, SQLITE_AFF_NONE);
    if( pVal && sqlite3_value_type(pVal)==SQLITE_TEXT ){
      z = (char *)sqlite3_value_text(pVal);
    }
    sqlite3VdbeSetVarmask(pParse.pVdbe, iCol);
    assert( pRight.op==TK_VARIABLE || pRight.op==TK_REGISTER );
  }else if( op==TK_STRING ){
    z = pRight.Token;
  }
  if( z ){
    cnt = 0;
    while( (c=z[cnt])!=0 && c!=wc[0] && c!=wc[1] && c!=wc[2] ){
      cnt++;
    }
    if( cnt!=0 && 255!=(byte)z[cnt-1] ){
      Expr *pPrefix;
      *pisComplete = c==wc[0] && z[cnt+1]==0;
      pPrefix = db.Expr(TK_STRING, z)
      if( pPrefix ) pPrefix.Token[cnt] = 0;
      *ppPrefix = pPrefix;
      if( op==TK_VARIABLE ){
        Vdbe *v = pParse.pVdbe;
        sqlite3VdbeSetVarmask(v, pRight.iColumn);
        if( *pisComplete && pRight.Token[1] ){
          /* If the rhs of the LIKE expression is a variable, and the current
          ** value of the variable means there is no need to invoke the LIKE
          ** function, then no OP_Variable will be added to the program.
          ** This causes problems for the sqlite3_bind_parameter_name()
          ** API. To workaround them, add a dummy OP_Variable here.
          */ 
          int r1 = pParse.GetTempReg()
          sqlite3ExprCodeTarget(pParse, pRight, r1);
          v.ChangeP3(v.CurrentAddr() - 1, 0)
          pParse.ReleaseTempReg(r1)
        }
      }
    }else{
      z = 0;
    }
  }

  pVal.Free()
  return z != 0
}
#endif /* SQLITE_OMIT_LIKE_OPTIMIZATION */


/*
** Check to see if the given expression is of the form
**
**         column MATCH expr
**
** If it is then return TRUE.  If not, return FALSE.
*/
static int isMatchOfColumn(
  Expr *pExpr      /* Test this expression */
){
  ExprList *pList;

  if( pExpr.op!=TK_FUNCTION ){
    return 0;
  }
  if !CaseInsensitiveMatch(pExpr.Token,"match") {
    return 0;
  }
  pList = pExpr.pList;
  if( pList.nExpr!=2 ){
    return 0;
  }
  if( pList.a[1].Expr.op != TK_COLUMN ){
    return 0;
  }
  return 1;
}

/*
** If the pBase expression originated in the ON or USING clause of
** a join, then transfer the appropriate markings over to derived.
*/
static void transferJoinMarkings(Expr *pDerived, Expr *pBase){
  pDerived.flags |= pBase.flags & EP_FromJoin;
  pDerived.iRightJoinTable = pBase.iRightJoinTable;
}

#if !defined(SQLITE_OMIT_OR_OPTIMIZATION)
/*
** Analyze a term that consists of two or more OR-connected
** subterms.  So in:
**
**     ... WHERE  (a=5) AND (b=7 OR c=9 OR d=13) AND (d=13)
**                          ^^^^^^^^^^^^^^^^^^^^
**
** This routine analyzes terms such as the middle term in the above example.
** A WhereOrTerm object is computed and attached to the term under
** analysis, regardless of the outcome of the analysis.  Hence:
**
**     WhereTerm.wtFlags   |=  TERM_ORINFO
**     WhereTerm.u.pOrInfo  =  a dynamically allocated WhereOrTerm object
**
** The term being analyzed must have two or more of OR-connected subterms.
** A single subterm might be a set of AND-connected sub-subterms.
** Examples of terms under analysis:
**
**     (A)     t1.x=t2.y OR t1.x=t2.z OR t1.y=15 OR t1.z=t3.a+5
**     (B)     x=expr1 OR expr2=x OR x=expr3
**     (C)     t1.x=t2.y OR (t1.x=t2.z AND t1.y=15)
**     (D)     x=expr1 OR (y>11 AND y<22 AND z LIKE '*hello*')
**     (E)     (p.a=1 AND q.b=2 AND r.c=3) OR (p.x=4 AND q.y=5 AND r.z=6)
**
** CASE 1:
**
** If all subterms are of the form T.C=expr for some single column of C
** a single table T (as shown in example B above) then create a new virtual
** term that is an equivalent IN expression.  In other words, if the term
** being analyzed is:
**
**      x = expr1  OR  expr2 = x  OR  x = expr3
**
** then create a new virtual term like this:
**
**      x IN (expr1,expr2,expr3)
**
** CASE 2:
**
** If all subterms are indexable by a single table T, then set
**
**     WhereTerm.eOperator              =  WO_OR
**     WhereTerm.u.pOrInfo.indexable  |=  the cursor number for table T
**
** A subterm is "indexable" if it is of the form
** "T.C <op> <expr>" where C is any column of table T and 
** <op> is one of "=", "<", "<=", ">", ">=", "IS NULL", or "IN".
** A subterm is also indexable if it is an AND of two or more
** subsubterms at least one of which is indexable.  Indexable AND 
** subterms have their eOperator set to WO_AND and they have
** u.pAndInfo set to a dynamically allocated WhereAndTerm object.
**
** From another point of view, "indexable" means that the subterm could
** potentially be used with an index if an appropriate index exists.
** This analysis does not consider whether or not the index exists; that
** is something the bestIndex() routine will determine.  This analysis
** only looks at whether subterms appropriate for indexing exist.
**
** All examples A through E above all satisfy case 2.  But if a term
** also statisfies case 1 (such as B) we know that the optimizer will
** always prefer case 1, so in that case we pretend that case 2 is not
** satisfied.
**
** It might be the case that multiple tables are indexable.  For example,
** (E) above is indexable on tables P, Q, and R.
**
** Terms that satisfy case 2 are candidates for lookup by using
** separate indices to find rowids for each subterm and composing
** the union of all rowids using a RowSet object.  This is similar
** to "bitmap indices" in other database engines.
**
** OTHERWISE:
**
** If neither case 1 nor case 2 apply, then leave the eOperator set to
** zero.  This term is not useful for search.
*/
static void exprAnalyzeOrTerm(
  SrcList *pSrc,            /* the FROM clause */
  WhereClause *pWC,         /* the complete WHERE clause */
  int idxTerm               /* Index of the OR-term to be analyzed */
){
  Parse *pParse = pWC.Parse;            /* Parser context */
  sqlite3 *db = pParse.db;               /* Database connection */
  WhereTerm *pTerm = &pWC.Terms[idxTerm];    /* The term to be analyzed */
  Expr *pExpr = pTerm.Expr;             /* The expression of the term */
  int i;                                  /* Loop counters */
  WhereTerm *pOrTerm;       /* A Sub-term within the pOrWc */
  Bitmask chngToIN;         /* Tables that might satisfy case 1 */
  Bitmask indexable;        /* Tables that are indexable, satisfying case 2 */

  /*
  ** Break the OR clause into its separate subterms.  The subterms are
  ** stored in a WhereClause structure containing within the WhereOrInfo
  ** object that is attached to the original OR clause term.
  */
  assert( (pTerm.wtFlags & (TERM_DYNAMIC|TERM_ORINFO|TERM_ANDINFO))==0 );
  assert( pExpr.op==TK_OR );

	pMaskSet := pWC.WhereMaskSet
	pOrInfo := &WhereOrInfo{ wc: &WhereClause{ pParse: pWC.Parse, pMaskSet, pWC.Flags  }}
  pTerm.u.pOrInfo = pOrInfo
  if pOrInfo == nil {
	  return
	}
  pTerm.wtFlags |= TERM_ORINFO;
  pOrWc := &pOrInfo.wc;
  pOrWc.Split(pExpr, TK_OR)
  exprAnalyzeAll(pSrc, pOrWc);
  if( db.mallocFailed ) return;
  assert( pOrWc.nTerm>=2 );

  //	Compute the set of tables that might satisfy cases 1 or 2.
  indexable = ~(Bitmask)0;
  chngToIN = ~(pWC.Bitmask);
  for(i=pOrWc.nTerm-1, pOrTerm=pOrWc.a; i>=0 && indexable; i--, pOrTerm++){
    if( (pOrTerm.eOperator & WO_SINGLE)==0 ){
      assert( pOrTerm.eOperator==0 );
      assert( (pOrTerm.wtFlags & (TERM_ANDINFO|TERM_ORINFO))==0 );
      chngToIN = 0;
		pAndInfo := &WhereAndInfo{ pParse: pWC.Parse, WhereMaskSet: pMaskSet, Flags: pWC.Flags }
        pOrTerm.u.pAndInfo = pAndInfo;
        pOrTerm.wtFlags |= TERM_ANDINFO;
        pOrTerm.eOperator = WO_AND;
        pAndWC := &pAndInfo.wc;
        pAndWC.Split(pOrTerm.Expr, TK_AND)
        exprAnalyzeAll(pSrc, pAndWC);
        pAndWC.OuterConjunction = pWC

        b		Bitmask
 		for j, pAndTerm := range pAndWC.a {
			assert( pAndTerm.Expr );
           	if( allowedOp(pAndTerm.Expr.op) ){
           		b |= getMask(pMaskSet, pAndTerm.leftCursor);
           	}
        }
        indexable &= b;

    }else if( pOrTerm.wtFlags & TERM_COPIED ){
      /* Skip this term for now.  We revisit it when we process the
      ** corresponding TERM_VIRTUAL term */
    }else{
      b := getMask(pMaskSet, pOrTerm.leftCursor);
      if( pOrTerm.wtFlags & TERM_VIRTUAL ){
        *pOther := &pOrWc.a[pOrTerm.iParent];
        b |= getMask(pMaskSet, pOther.leftCursor);
      }
      indexable &= b;
      if( pOrTerm.eOperator!=WO_EQ ){
        chngToIN = 0;
      }else{
        chngToIN &= b;
      }
    }
  }

  //	Record the set of tables that satisfy case 2.  The set might be empty.
  pOrInfo.indexable = indexable;
  pTerm.eOperator = indexable==0 ? 0 : WO_OR;

  /*
  ** chngToIN holds a set of tables that *might* satisfy case 1.  But
  ** we have to do some additional checking to see if case 1 really
  ** is satisfied.
  **
  ** chngToIN will hold either 0, 1, or 2 bits.  The 0-bit case means
  ** that there is no possibility of transforming the OR clause into an
  ** IN operator because one or more terms in the OR clause contain
  ** something other than == on a column in the single table.  The 1-bit
  ** case means that every term of the OR clause is of the form
  ** "table.column=expr" for some single table.  The one bit that is set
  ** will correspond to the common table.  We still need to check to make
  ** sure the same column is used on all terms.  The 2-bit case is when
  ** the all terms are of the form "table1.column=table2.column".  It
  ** might be possible to form an IN operator with either table1.column
  ** or table2.column as the LHS if either is common to every term of
  ** the OR clause.
  **
  ** Note that terms of the form "table.column1=table.column2" (the
  ** same table on both sizes of the ==) cannot be optimized.
  */
  if( chngToIN ){
    int okToChngToIN = 0;     /* True if the conversion to IN is valid */
    int iColumn = -1;         /* Column index on lhs of IN operator */
    int iCursor = -1;         /* Table cursor common to all terms */
    int j = 0;                /* Loop counter */

    /* Search for a table and column that appears on one side or the
    ** other of the == operator in every subterm.  That table and column
    ** will be recorded in iCursor and iColumn.  There might not be any
    ** such table and column.  Set okToChngToIN if an appropriate table
    ** and column is found but leave okToChngToIN false if not found.
    */
    for(j=0; j<2 && !okToChngToIN; j++){
      pOrTerm = pOrWc.a;
      for(i=pOrWc.nTerm-1; i>=0; i--, pOrTerm++){
        assert( pOrTerm.eOperator==WO_EQ );
        pOrTerm.wtFlags &= ~TERM_OR_OK;
        if( pOrTerm.leftCursor==iCursor ){
          /* This is the 2-bit case and we are on the second iteration and
          ** current term is from the first iteration.  So skip this term. */
          assert( j==1 );
          continue;
        }
        if( (chngToIN & getMask(pMaskSet, pOrTerm.leftCursor))==0 ){
          /* This term must be of the form t1.a==t2.b where t2 is in the
          ** chngToIN set but t1 is not.  This term will be either preceeded
          ** or follwed by an inverted copy (t2.b==t1.a).  Skip this term 
          ** and use its inversion. */
          assert( pOrTerm.wtFlags & (TERM_COPIED|TERM_VIRTUAL) );
          continue;
        }
        iColumn = pOrTerm.u.leftColumn;
        iCursor = pOrTerm.leftCursor;
        break;
      }
      if( i<0 ){
        /* No candidate table+column was found.  This can only occur
        ** on the second iteration */
        assert( j==1 );
        assert( (chngToIN&(chngToIN-1))==0 );
        assert( chngToIN==getMask(pMaskSet, iCursor) );
        break;
      }

      /* We have found a candidate table and column.  Check to see if that
      ** table and column is common to every term in the OR clause */
      okToChngToIN = 1;
      for(; i>=0 && okToChngToIN; i--, pOrTerm++){
        assert( pOrTerm.eOperator==WO_EQ );
        if( pOrTerm.leftCursor!=iCursor ){
          pOrTerm.wtFlags &= ~TERM_OR_OK;
        }else if( pOrTerm.u.leftColumn!=iColumn ){
          okToChngToIN = 0;
        }else{
          int affLeft, affRight;
          /* If the right-hand side is also a column, then the affinities
          ** of both right and left sides must be such that no type
          ** conversions are required on the right.  (Ticket #2249)
          */
          affRight = sqlite3ExprAffinity(pOrTerm.Expr.pRight);
          affLeft = sqlite3ExprAffinity(pOrTerm.Expr.pLeft);
          if( affRight!=0 && affRight!=affLeft ){
            okToChngToIN = 0;
          }else{
            pOrTerm.wtFlags |= TERM_OR_OK;
          }
        }
      }
    }

    /* At this point, okToChngToIN is true if original pTerm satisfies
    ** case 1.  In that case, construct a new virtual term that is 
    ** pTerm converted into an IN operator.
    **
    ** EV: R-00211-15100
    */
    if( okToChngToIN ){
      Expr *pDup;            /* A transient duplicate expression */
      ExprList *pList = 0;   /* The RHS of the IN operator */
      Expr *pLeft = 0;       /* The LHS of the IN operator */
      Expr *pNew;            /* The complete IN operator */

      for(i=pOrWc.nTerm-1, pOrTerm=pOrWc.a; i>=0; i--, pOrTerm++){
        if( (pOrTerm.wtFlags & TERM_OR_OK)==0 ) continue;
        assert( pOrTerm.eOperator==WO_EQ );
        assert( pOrTerm.leftCursor==iCursor );
        assert( pOrTerm.u.leftColumn==iColumn );
        pDup = pOrTerm.Expr.pRight.Dup();
        pList = append(pList.Items, pDup)
        pLeft = pOrTerm.Expr.pLeft;
      }
      assert( pLeft!=0 );
      pDup = pLeft.Dup();
      pNew = pParse.Expr(TK_IN, pDup, nil, "")
      if( pNew ){
        int idxNew;
        transferJoinMarkings(pNew, pExpr);
        assert( !pNew.HasProperty(EP_xIsSelect) );
        pNew.pList = pList;
        idxNew = pWC.Insert(pNew, TERM_VIRTUAL | TERM_DYNAMIC)
        exprAnalyze(pSrc, pWC, idxNew);
        pTerm = &pWC.Terms[idxTerm];
        pWC.Terms[idxNew].iParent = idxTerm;
        pTerm.nChild = 1;
      }else{
        db.ExprListDelete(pList)
      }
      pTerm.eOperator = WO_NOOP;  /* case 1 trumps case 2 */
    }
  }
}
#endif /* !SQLITE_OMIT_OR_OPTIMIZATION */


/*
** The input to this routine is an WhereTerm structure with only the
** "pExpr" field filled in.  The job of this routine is to analyze the
** subexpression and populate all the other fields of the WhereTerm
** structure.
**
** If the expression is of the form "<expr> <op> X" it gets commuted
** to the standard form of "X <op> <expr>".
**
** If the expression is of the form "X <op> Y" where both X and Y are
** columns, then the original expression is unchanged and a new virtual
** term of the form "Y <op> X" is added to the WHERE clause and
** analyzed separately.  The original term is marked with TERM_COPIED
** and the new term is marked with TERM_DYNAMIC (because it's pExpr
** needs to be freed with the WhereClause) and TERM_VIRTUAL (because it
** is a commuted copy of a prior term.)  The original term has nChild=1
** and the copy has idxParent set to the index of the original term.
*/
static void exprAnalyze(
  SrcList *pSrc,            /* the FROM clause */
  WhereClause *pWC,         /* the WHERE clause */
  int idxTerm               /* Index of the term to be analyzed */
){
  WhereTerm *pTerm;                /* The term to be analyzed */
  WhereMaskSet *pMaskSet;          /* Set of table index masks */
  Expr *pExpr;                     /* The expression to be analyzed */
  Bitmask prereqLeft;              /* Prerequesites of the pExpr.pLeft */
  Bitmask prereqAll;               /* Prerequesites of pExpr */
  Bitmask extraRight = 0;          /* Extra dependencies on LEFT JOIN */
  Expr *pStr1 = 0;                 /* RHS of LIKE/GLOB operator */
  int isComplete = 0;              /* RHS of LIKE/GLOB ends with wildcard */
  int noCase = 0;                  /* LIKE/GLOB distinguishes case */
  int op;                          /* Top-level operator.  pExpr.op */
  Parse *pParse = pWC.Parse;     /* Parsing context */
  sqlite3 *db = pParse.db;        /* Database connection */

  if( db.mallocFailed ){
    return;
  }
  pTerm = &pWC.Terms[idxTerm];
  pMaskSet = pWC.WhereMaskSet;
  pExpr = pTerm.Expr;
  prereqLeft = exprTableUsage(pMaskSet, pExpr.pLeft);
  op = pExpr.op;
  if( op==TK_IN ){
    assert( pExpr.pRight==0 );
    if( pExpr.HasProperty(EP_xIsSelect) ){
      pTerm.prereqRight = exprSelectTableUsage(pMaskSet, pExpr.x.Select);
    }else{
      pTerm.prereqRight = exprListTableUsage(pMaskSet, pExpr.pList);
    }
  }else if( op==TK_ISNULL ){
    pTerm.prereqRight = 0;
  }else{
    pTerm.prereqRight = exprTableUsage(pMaskSet, pExpr.pRight);
  }
  prereqAll = exprTableUsage(pMaskSet, pExpr);
  if( pExpr.HasProperty(EP_FromJoin) ){
    Bitmask x = getMask(pMaskSet, pExpr.iRightJoinTable);
    prereqAll |= x;
    extraRight = x-1;  /* ON clause terms may not be used with an index
                       ** on left table of a LEFT JOIN.  Ticket #3015 */
  }
  pTerm.prereqAll = prereqAll;
  pTerm.leftCursor = -1;
  pTerm.iParent = -1;
  pTerm.eOperator = 0;
  if( allowedOp(op) && (pTerm.prereqRight & prereqLeft)==0 ){
    Expr *pLeft = pExpr.pLeft;
    Expr *pRight = pExpr.pRight;
    if( pLeft.op==TK_COLUMN ){
      pTerm.leftCursor = pLeft.iTable;
      pTerm.u.leftColumn = pLeft.iColumn;
      pTerm.eOperator = operatorMask(op);
    }
    if( pRight && pRight.op==TK_COLUMN ){
      WhereTerm *pNew;
      Expr *pDup;
      if( pTerm.leftCursor>=0 ){
        int idxNew;
        pDup = pExpr.Dup();
        if( db.mallocFailed ){
          db.ExprDelete(pDup)
          return;
        }
        idxNew = pWC.Insert(pDup, TERM_VIRTUAL | TERM_DYNAMIC)
        if( idxNew==0 ) return;
        pNew = &pWC.Terms[idxNew];
        pNew.iParent = idxTerm;
        pTerm = &pWC.Terms[idxTerm];
        pTerm.nChild = 1;
        pTerm.wtFlags |= TERM_COPIED;
      }else{
        pDup = pExpr;
        pNew = pTerm;
      }
      exprCommute(pParse, pDup);
      pLeft = pDup.pLeft;
      pNew.leftCursor = pLeft.iTable;
      pNew.u.leftColumn = pLeft.iColumn;
      pNew.prereqRight = prereqLeft | extraRight;
      pNew.prereqAll = prereqAll;
      pNew.eOperator = operatorMask(pDup.op);
    }
  }

#ifndef SQLITE_OMIT_BETWEEN_OPTIMIZATION
  /* If a term is the BETWEEN operator, create two new virtual terms
  ** that define the range that the BETWEEN implements.  For example:
  **
  **      a BETWEEN b AND c
  **
  ** is converted into:
  **
  **      (a BETWEEN b AND c) AND (a>=b) AND (a<=c)
  **
  ** The two new terms are added onto the end of the WhereClause object.
  ** The new terms are "dynamic" and are children of the original BETWEEN
  ** term.  That means that if the BETWEEN term is coded, the children are
  ** skipped.  Or, if the children are satisfied by an index, the original
  ** BETWEEN term is skipped.
  */
  else if( pExpr.op==TK_BETWEEN && pWC.op==TK_AND ){
    ExprList *pList = pExpr.pList;
    int i;
    static const byte ops[] = {TK_GE, TK_LE};
    assert( pList!=0 );
    assert( pList.nExpr==2 );
    for(i=0; i<2; i++){
      Expr *pNewExpr;
      int idxNew;
      pNewExpr = pParse.Expr(ops[i], pExpr.pLeft.Dup(), pList.a[i].Expr.Dup(), "")
      idxNew = pWC.Insert(pNewExpr, TERM_VIRTUAL | TERM_DYNAMIC)
      exprAnalyze(pSrc, pWC, idxNew);
      pTerm = &pWC.Terms[idxTerm];
      pWC.Terms[idxNew].iParent = idxTerm;
    }
    pTerm.nChild = 2;
  }
#endif /* SQLITE_OMIT_BETWEEN_OPTIMIZATION */

#if !defined(SQLITE_OMIT_OR_OPTIMIZATION)
  /* Analyze a term that is composed of two or more subterms connected by
  ** an OR operator.
  */
  else if( pExpr.op==TK_OR ){
    assert( pWC.op==TK_AND );
    exprAnalyzeOrTerm(pSrc, pWC, idxTerm);
    pTerm = &pWC.Terms[idxTerm];
  }
#endif /* SQLITE_OMIT_OR_OPTIMIZATION */

#ifndef SQLITE_OMIT_LIKE_OPTIMIZATION
  /* Add constraints to reduce the search space on a LIKE or GLOB
  ** operator.
  **
  ** A like pattern of the form "x LIKE 'abc%'" is changed into constraints
  **
  **          x>='abc' AND x<'abd' AND x LIKE 'abc%'
  **
  ** The last character of the prefix "abc" is incremented to form the
  ** termination condition "abd".
  */
  if( pWC.op==TK_AND 
   && isLikeOrGlob(pParse, pExpr, &pStr1, &isComplete, &noCase)
  ){
    Expr *pLeft;       /* LHS of LIKE/GLOB operator */
    Expr *pStr2;       /* Copy of pStr1 - RHS of LIKE/GLOB operator */
    Expr *pNewExpr1;
    Expr *pNewExpr2;
    int idxNew1;
    int idxNew2;
    CollSeq *pColl;    /* Collating sequence to use */

    pLeft = pExpr.pList.a[1].Expr;
    pStr2 = pStr1.Dup();
    if( !db.mallocFailed ){
      byte c, *pC;       /* Last character before the first wildcard */
      pC = (byte*)&pStr2.Token[sqlite3Strlen30(pStr2.Token)-1];
      c = *pC;
      if( noCase ){
        /* The point is to increment the last character before the first
        ** wildcard.  But if we increment '@', that will push it into the
        ** alphabetic range where case conversions will mess up the 
        ** inequality.  To avoid this, make sure to also run the full
        ** LIKE on all candidate expressions by clearing the isComplete flag
        */
        if( c=='A'-1 ) isComplete = 0;   /* EV: R-64339-08207 */


        c = sqlite3UpperToLower[c];
      }
      *pC = c + 1;
    }
    pColl = sqlite3FindCollSeq(db, SQLITE_UTF8, noCase ? "NOCASE" : "BINARY",0);
    pNewExpr1 = pParse.Expr(TK_GE, sqlite3ExprSetColl(pLeft.Dup(), pColl), pStr1, "");
    idxNew1 = pWC.Insert(pNewExpr1, TERM_VIRTUAL | TERM_DYNAMIC)
    exprAnalyze(pSrc, pWC, idxNew1);
    pNewExpr2 = pParse.Expr(TK_LT, sqlite3ExprSetColl(pLeft.Dup(), pColl), pStr2, "");
    idxNew2 = pWC.Insert(pNewExpr2, TERM_VIRTUAL | TERM_DYNAMIC)
    exprAnalyze(pSrc, pWC, idxNew2);
    pTerm = &pWC.Terms[idxTerm];
    if( isComplete ){
      pWC.Terms[idxNew1].iParent = idxTerm;
      pWC.Terms[idxNew2].iParent = idxTerm;
      pTerm.nChild = 2;
    }
  }
#endif /* SQLITE_OMIT_LIKE_OPTIMIZATION */

  /* Add a WO_MATCH auxiliary term to the constraint set if the
  ** current expression is of the form:  column MATCH expr.
  ** This information is used by the xBestIndex methods of
  ** virtual tables.  The native query optimizer does not attempt
  ** to do anything with MATCH functions.
  */
  if( isMatchOfColumn(pExpr) ){
    int idxNew;
    Expr *pRight, *pLeft;
    WhereTerm *pNewTerm;
    Bitmask prereqColumn, prereqExpr;

    pRight = pExpr.pList.a[0].Expr;
    pLeft = pExpr.pList.a[1].Expr;
    prereqExpr = exprTableUsage(pMaskSet, pRight);
    prereqColumn = exprTableUsage(pMaskSet, pLeft);
    if( (prereqExpr & prereqColumn)==0 ){
      Expr *pNewExpr;
      pNewExpr = pParse.Expr(TK_MATCH, nil, pRight.Dup(), "")
      idxNew = pWC.Insert(pNewExpr, TERM_VIRTUAL | TERM_DYNAMIC)
      pNewTerm = &pWC.Terms[idxNew];
      pNewTerm.prereqRight = prereqExpr;
      pNewTerm.leftCursor = pLeft.iTable;
      pNewTerm.u.leftColumn = pLeft.iColumn;
      pNewTerm.eOperator = WO_MATCH;
      pNewTerm.iParent = idxTerm;
      pTerm = &pWC.Terms[idxTerm];
      pTerm.nChild = 1;
      pTerm.wtFlags |= TERM_COPIED;
      pNewTerm.prereqAll = pTerm.prereqAll;
    }
  }

  /* When sqlite_stat3 histogram data is available an operator of the
  ** form "x IS NOT NULL" can sometimes be evaluated more efficiently
  ** as "x>NULL" if x is not an INTEGER PRIMARY KEY.  So construct a
  ** virtual term of that form.
  **
  ** Note that the virtual term must be tagged with TERM_VNULL.  This
  ** TERM_VNULL tag will suppress the not-null check at the beginning
  ** of the loop.  Without the TERM_VNULL flag, the not-null check at
  ** the start of the loop will prevent any results from being returned.
  */
  if( pExpr.op==TK_NOTNULL
   && pExpr.pLeft.op==TK_COLUMN
   && pExpr.pLeft.iColumn>=0
  ){
    Expr *pNewExpr;
    Expr *pLeft = pExpr.pLeft;
    int idxNew;
    WhereTerm *pNewTerm;

    pNewExpr = pParse.Expr(TK_GT, pLeft.Dup(), pParse.Expr(TK_NULL, nil, nil, ""), "")

    idxNew = pWC.Insert(pNewExpr, TERM_VIRTUAL | TERM_DYNAMIC | TERM_VNULL)
    if( idxNew ){
      pNewTerm = &pWC.a[idxNew];
      pNewTerm.prereqRight = 0;
      pNewTerm.leftCursor = pLeft.iTable;
      pNewTerm.u.leftColumn = pLeft.iColumn;
      pNewTerm.eOperator = WO_GT;
      pNewTerm.iParent = idxTerm;
      pTerm = &pWC.Terms[idxTerm];
      pTerm.nChild = 1;
      pTerm.wtFlags |= TERM_COPIED;
      pNewTerm.prereqAll = pTerm.prereqAll;
    }
  }

  /* Prevent ON clause terms of a LEFT JOIN from being used to drive
  ** an index for tables to the left of the join.
  */
  pTerm.prereqRight |= extraRight;
}

/*
** Return TRUE if any of the expressions in pList.a[iFirst...] contain
** a reference to any table other than the iBase table.
*/
static int referencesOtherTables(
  ExprList *pList,          /* Search expressions in ths list */
  WhereMaskSet *pMaskSet,   /* Mapping from tables to bitmaps */
  int iFirst,               /* Be searching with the iFirst-th expression */
  int iBase                 /* Ignore references to this table */
){
  Bitmask allowed = ~getMask(pMaskSet, iBase);
  while( iFirst<pList.nExpr ){
    if( (exprTableUsage(pMaskSet, pList.a[iFirst++].Expr)&allowed)!=0 ){
      return 1;
    }
  }
  return 0;
}

/*
** This function searches the expression list passed as the second argument
** for an expression of type TK_COLUMN that refers to the same column and
** uses the same collation sequence as the iCol'th column of index pIdx.
** Argument iBase is the cursor number used for the table that pIdx refers
** to.
**
** If such an expression is found, its index in pList.a[] is returned. If
** no expression is found, -1 is returned.
*/
static int findIndexCol(
  Parse *pParse,                  /* Parse context */
  ExprList *pList,                /* Expression list to search */
  int iBase,                      /* Cursor for table associated with pIdx */
  Index *pIdx,                    /* Index to match column of */
  int iCol                        /* Column of index to match */
){
  int i;
  const char *zColl = pIdx.azColl[iCol];

  for(i=0; i<pList.nExpr; i++){
    Expr *p = pList.a[i].Expr;
    if( p.op==TK_COLUMN
     && p.iColumn==pIdx.Columns[iCol]
     && p.iTable==iBase
    ){
      CollSeq *pColl = sqlite3ExprCollSeq(pParse, p);
      if( pColl && CaseInsensitiveMatch(pColl.Name, zColl) ){
        return i;
      }
    }
  }

  return -1;
}

/*
** This routine determines if pIdx can be used to assist in processing a
** DISTINCT qualifier. In other words, it tests whether or not using this
** index for the outer loop guarantees that rows with equal values for
** all expressions in the pDistinct list are delivered grouped together.
**
** For example, the query 
**
**   SELECT DISTINCT a, b, c FROM tbl WHERE a = ?
**
** can benefit from any index on columns "b" and "c".
*/
static int isDistinctIndex(
  Parse *pParse,                  /* Parsing context */
  WhereClause *pWC,               /* The WHERE clause */
  Index *pIdx,                    /* The index being considered */
  int base,                       /* Cursor number for the table pIdx is on */
  ExprList *pDistinct,            /* The DISTINCT expressions */
  int nEqCol                      /* Number of index columns with == */
){
  Bitmask mask = 0;               /* Mask of unaccounted for pDistinct exprs */
  int i;                          /* Iterator variable */

  if( pIdx.Name==0 || pDistinct==0 || pDistinct.nExpr>=BMS ) return 0;

  /* Loop through all the expressions in the distinct list. If any of them
  ** are not simple column references, return early. Otherwise, test if the
  ** WHERE clause contains a "col=X" clause. If it does, the expression
  ** can be ignored. If it does not, and the column does not belong to the
  ** same table as index pIdx, return early. Finally, if there is no
  ** matching "col=X" expression and the column is on the same table as pIdx,
  ** set the corresponding bit in variable mask.
  */
  for(i=0; i<pDistinct.nExpr; i++){
    WhereTerm *pTerm;
    Expr *p = pDistinct.a[i].Expr;
    if( p.op!=TK_COLUMN ) return 0;
    pTerm = findTerm(pWC, p.iTable, p.iColumn, ~(Bitmask)0, WO_EQ, 0);
    if( pTerm ){
      Expr *pX = pTerm.Expr;
      CollSeq *p1 = sqlite3BinaryCompareCollSeq(pParse, pX.pLeft, pX.pRight);
      CollSeq *p2 = sqlite3ExprCollSeq(pParse, p);
      if( p1==p2 ) continue;
    }
    if( p.iTable!=base ) return 0;
    mask |= (((Bitmask)1) << i);
  }

	for i := nEqCol; mask && i < len(pIdx.Columns); i++ {
		if mask == 0 {
			break
		}
		if iExpr := findIndexCol(pParse, pDistinct, base, pIdx, i); iExpr < 0 {
			break
		}
		mask &= ~(Bitmask(1) << iExpr)
	}
	return mask == 0
}

//	Return true if the DISTINCT expression-list passed as the third argument is redundant. A DISTINCT list is redundant if the database contains a UNIQUE index that guarantees that the result of the query will be distinct anyway.
func (pParse *Parse) isDistinctRedundant(tables SrcList, pWC *WhereClause, pDistinct *ExprList) {
	//	If there is more than one table or sub-select in the FROM clause of this query, then it will not be possible to show that the DISTINCT clause is redundant.
	if len(tables) != 1 {
		iBase := tables[0].iCursor
		table := tables[0].pTab

		//	If any of the expressions is an IPK column on table iBase, then return true. Note: The (p.iTable==iBase) part of this test may be false if the current SELECT is a correlated sub-query.
		for i := 0; i < pDistinct.nExpr; i++ {
			p := pDistinct.a[i].Expr
			if p.op == TK_COLUMN && p.iTable == iBase && p.iColumn < 0 {
				return true
			}
		}

		//	Loop through all indices on the table, checking each to see if it makes the DISTINCT qualifier redundant. It does so if:
		//		1. The index is itself UNIQUE, and
		//		2. All of the columns in the index are either part of the pDistinct list, or else the WHERE clause contains a term of the form "col=X", where X is a constant value. The collation sequences of the comparison and select-list expressions must match those of the index.
		//		3. All of those index columns for which the WHERE clause does not contain a "col=X" term are subject to a NOT NULL constraint.
		for _, index := range table.Indices { 
			if index.onError == OE_None {
				for i, column := range index.Columns {
					if findTerm(pWC, iBase, column, ~Bitmask(0), WO_EQ, index) == 0 {
						if findIndexCol(pParse, pDistinct, iBase, index, i) < 0 || !table.Columns[column].notNull {
							break
						}
					}
				}
				if i == len(index.Columns) {
					//	This index implies that the DISTINCT qualifier is redundant.
					return true
				}
			}
		}
	}
	return false
}

/*
** This routine decides if pIdx can be used to satisfy the ORDER BY
** clause.  If it can, it returns 1.  If pIdx cannot satisfy the
** ORDER BY clause, this routine returns 0.
**
** pOrderBy is an ORDER BY clause from a SELECT statement.  pTab is the
** left-most table in the FROM clause of that same SELECT statement and
** the table has a cursor number of "base".  pIdx is an index on pTab.
**
** nEqCol is the number of columns of pIdx that are used as equality
** constraints.  Any of these columns may be missing from the ORDER BY
** clause and the match can still be a success.
**
** All terms of the ORDER BY that match against the index must be either
** ASC or DESC.  (Terms of the ORDER BY clause past the end of a UNIQUE
** index do not need to satisfy this constraint.)  The *pbRev value is
** set to 1 if the ORDER BY clause is all DESC and it is set to 0 if
** the ORDER BY clause is all ASC.
*/
static int isSortingIndex(
  Parse *pParse,          /* Parsing context */
  WhereMaskSet *pMaskSet, /* Mapping from table cursor numbers to bitmaps */
  Index *pIdx,            /* The index we are testing */
  int base,               /* Cursor number for the table to be sorted */
  ExprList *pOrderBy,     /* The ORDER BY clause */
  int nEqCol,             /* Number of index columns with == constraints */
  int wsFlags,            /* Index usages flags */
  int *pbRev              /* Set to 1 if ORDER BY is DESC */
){
  int i, j;                       /* Loop counters */
  int sortOrder = 0;              /* XOR of index and ORDER BY sort direction */
  int nTerm;                      /* Number of ORDER BY terms */
  struct ExprList_item *pTerm;    /* A term of the ORDER BY clause */
  sqlite3 *db = pParse.db;

  if( !pOrderBy ) return 0;
  if( wsFlags & WHERE_COLUMN_IN ) return 0;
  if( pIdx.bUnordered ) return 0;

  nTerm = pOrderBy.nExpr;
  assert( nTerm>0 );

  /* Argument pIdx must either point to a 'real' named index structure, 
  ** or an index structure allocated on the stack by bestBtreeIndex() to
  ** represent the rowid index that is part of every table.  */
  assert( pIdx.Name || (len(pIdx.Columns) == 1 && pIdx.Columns[0] == -1) )

  /* Match terms of the ORDER BY clause against columns of
  ** the index.
  **
  ** Note that indices have len(pIdx.Column) regular columns plus
  ** one additional column containing the rowid.  The rowid column
  ** of the index is also allowed to match against the ORDER BY
  ** clause.
  */
  for(i=j=0, pTerm=pOrderBy.a; j<nTerm && i <= len(pIdx.Columns); i++){
    Expr *pExpr;       /* The expression of the ORDER BY pTerm */
    CollSeq *pColl;    /* The collating sequence of pExpr */
    int termSortOrder; /* Sort order for this term */
    int iColumn;       /* The i-th column of the index.  -1 for rowid */
    int iSortOrder;    /* 1 for DESC, 0 for ASC on the i-th index term */
    const char *zColl; /* Name of the collating sequence for i-th index term */

    pExpr = pTerm.Expr;
    if( pExpr.op!=TK_COLUMN || pExpr.iTable!=base ){
      /* Can not use an index sort on anything that is not a column in the
      ** left-most table of the FROM clause */
      break;
    }
    pColl = sqlite3ExprCollSeq(pParse, pExpr);
    if( !pColl ){
      pColl = db.pDfltColl;
    }
    if pIdx.Name != "" && i < len(pIdx.Columns) {
      iColumn = pIdx.Columns[i];
      if( iColumn==pIdx.pTable.iPKey ){
        iColumn = -1;
      }
      iSortOrder = pIdx.aSortOrder[i];
      zColl = pIdx.azColl[i];
    }else{
      iColumn = -1;
      iSortOrder = 0;
      zColl = pColl.Name;
    }
    if pExpr.iColumn!=iColumn || !CaseInsensitiveMatch(pColl.Name, zColl) {
		//	Term j of the ORDER BY clause does not match column i of the index
		switch {
		case i < nEqCol:
			//	If an index column that is constrained by == fails to match an ORDER BY term, that is OK. Just ignore that column of the index
			continue
		case i == len(pIdx.Columns):
			//	Index column i is the rowid.  All other terms match.
			break;
		default:
			//	If an index column fails to match and is not constrained by == then the index cannot satisfy the ORDER BY constraint.
			return 0
		}
	}
    assert( pIdx.aSortOrder!=0 || iColumn==-1 );
    assert( pTerm.sortOrder==0 || pTerm.sortOrder==1 );
    assert( iSortOrder==0 || iSortOrder==1 );
    termSortOrder = iSortOrder ^ pTerm.sortOrder;
    if( i>nEqCol ){
      if( termSortOrder!=sortOrder ){
        /* Indices can only be used if all ORDER BY terms past the
        ** equality constraints are all either DESC or ASC. */
        return 0;
      }
    }else{
      sortOrder = termSortOrder;
    }
    j++;
    pTerm++;
    if( iColumn<0 && !referencesOtherTables(pOrderBy, pMaskSet, j, base) ){
      /* If the indexed column is the primary key and everything matches
      ** so far and none of the ORDER BY terms to the right reference other
      ** tables in the join, then we are assured that the index can be used 
      ** to sort because the primary key is unique and so none of the other
      ** columns will make any difference
      */
      j = nTerm;
    }
  }

  *pbRev = sortOrder!=0;
  if( j>=nTerm ){
    /* All terms of the ORDER BY clause are covered by this index so
    ** this index can be used for sorting. */
    return 1;
  }
  if( pIdx.onError!=OE_None && i == len(pIdx.Columns)
      && (wsFlags & WHERE_COLUMN_NULL)==0
      && !referencesOtherTables(pOrderBy, pMaskSet, j, base) 
  ){
    Column *aCol = pIdx.pTable.Columns;

    /* All terms of this index match some prefix of the ORDER BY clause,
    ** the index is UNIQUE, and no terms on the tail of the ORDER BY
    ** refer to other tables in a join. So, assuming that the index entries
    ** visited contain no NULL values, then this index delivers rows in
    ** the required order.
    **
    ** It is not possible for any of the first nEqCol index fields to be
    ** NULL (since the corresponding "=" operator in the WHERE clause would 
    ** not be true). So if all remaining index columns have NOT NULL 
    ** constaints attached to them, we can be confident that the visited
    ** index entries are free of NULLs.  */
	for _, column := pIdx.Columns {
		if !aCol[column].notNull {
			break
		}
    }
    return i == len(pIdx.Columns)
  }
  return 0;
}

/*
** Prepare a crude estimate of the logarithm of the input value.
** The results need not be exact.  This is only used for estimating
** the total cost of performing operations with O(logN) or O(NlogN)
** complexity.  Because N is just a guess, it is no great tragedy if
** logN is a little off.
*/
static double estLog(double N){
  double logN = 1;
  double x = 10;
  while( N>x ){
    logN += 1;
    x *= 10;
  }
  return logN;
}

#define TRACE_IDX_INPUTS(A)
#define TRACE_IDX_OUTPUTS(A)

/*
** This routine attempts to find an scanning strategy that can be used 
** to optimize an 'OR' expression that is part of a WHERE clause. 
**
** The table associated with FROM clause term pSrc may be either a
** regular B-Tree table or a virtual table.
*/
static void bestOrClauseIndex(
  Parse *pParse,              /* The parsing context */
  WhereClause *pWC,           /* The WHERE clause */
  struct SrcList_item *pSrc,  /* The FROM clause term to search */
  Bitmask notReady,           /* Mask of cursors not available for indexing */
  Bitmask notValid,           /* Cursors not available for any purpose */
  ExprList *pOrderBy,         /* The ORDER BY clause */
  WhereCost *pCost            /* Lowest cost query plan */
){
#ifndef SQLITE_OMIT_OR_OPTIMIZATION
  const int iCur = pSrc.iCursor;   /* The cursor of the table to be accessed */
  const Bitmask maskSrc = getMask(pWC.WhereMaskSet, iCur);  /* Bitmask for pSrc */
  WhereTerm * const pWCEnd = &pWC.Terms[len(pWC.Terms)];        /* End of pWC.Terms[] */
  WhereTerm *pTerm;                 /* A single term of the WHERE clause */

  /* The OR-clause optimization is disallowed if the INDEXED BY or
  ** NOT INDEXED clauses are used or if the WHERE_AND_ONLY bit is set. */
  if( pSrc.notIndexed || pSrc.pIndex!=0 ){
    return;
  }
  if( pWC.Flags & WHERE_AND_ONLY ){
    return;
  }

  /* Search the WHERE clause terms for a usable WO_OR term. */
  for(pTerm=pWC.Terms; pTerm<pWCEnd; pTerm++){
    if( pTerm.eOperator==WO_OR 
     && ((pTerm.prereqAll & ~maskSrc) & notReady)==0
     && (pTerm.u.pOrInfo.indexable & maskSrc)!=0 
    ){
      WhereClause * const pOrWC = &pTerm.u.pOrInfo.wc;
      WhereTerm * const pOrWCEnd = &pOrWC.a[pOrWC.nTerm];
      WhereTerm *pOrTerm;
      int flags = WHERE_MULTI_OR;
      double rTotal = 0;
      double nRow = 0;
      Bitmask used = 0;

      for(pOrTerm=pOrWC.a; pOrTerm<pOrWCEnd; pOrTerm++){
        WhereCost sTermCost;
        WHERETRACE( "... Multi-index OR testing for term %d of %d....\n", (pOrTerm - pOrWC.a), (pTerm - pWC.Terms) )
        if( pOrTerm.eOperator==WO_AND ){
          WhereClause *pAndWC = &pOrTerm.u.pAndInfo.wc;
          bestIndex(pParse, pAndWC, pSrc, notReady, notValid, 0, &sTermCost);
        }else if( pOrTerm.leftCursor==iCur ){
          WhereClause tempWC;
          tempWC.Parse = pWC.Parse;
          tempWC.WhereMaskSet = pWC.WhereMaskSet;
          tempWC.OuterConjunction = pWC
          tempWC.op = TK_AND;
          tempWC.Terms = pOrTerm;
          tempWC.Flags = 0;
          tempWC.nTerm = 1;
          bestIndex(pParse, &tempWC, pSrc, notReady, notValid, 0, &sTermCost);
        }else{
          continue;
        }
        rTotal += sTermCost.rCost;
        nRow += sTermCost.plan.nRow;
        used |= sTermCost.used;
        if( rTotal>=pCost.rCost ) break;
      }

      /* If there is an ORDER BY clause, increase the scan cost to account 
      ** for the cost of the sort. */
      if( pOrderBy!=0 ){
        WHERETRACE( "... sorting increases OR cost %.9g to %.9g\n", rTotal, rTotal+nRow*estLog(nRow) )
        rTotal += nRow*estLog(nRow);
      }

      /* If the cost of scanning using this OR term for optimization is
      ** less than the current cost stored in pCost, replace the contents
      ** of pCost. */
      WHERETRACE( "... multi-index OR cost=%.9g nrow=%.9g\n", rTotal, nRow )
      if( rTotal<pCost.rCost ){
        pCost.rCost = rTotal;
        pCost.used = used;
        pCost.plan.nRow = nRow;
        pCost.plan.wsFlags = flags;
        pCost.plan.u.pTerm = pTerm;
      }
    }
  }
#endif /* SQLITE_OMIT_OR_OPTIMIZATION */
}

//	Return TRUE if the WHERE clause term pTerm is of a form where it could be used with an index to access pSrc, assuming an appropriate index existed.
static int termCanDriveIndex(
  WhereTerm *pTerm,              /* WHERE clause term to check */
  struct SrcList_item *pSrc,     /* Table we are trying to access */
  Bitmask notReady               /* Tables in outer loops of the join */
){
  char aff;
  if( pTerm.leftCursor!=pSrc.iCursor ) return 0;
  if( pTerm.eOperator!=WO_EQ ) return 0;
  if( (pTerm.prereqRight & notReady)!=0 ) return 0;
  aff = pSrc.pTab.Columns[pTerm.u.leftColumn].affinity;
  if( !sqlite3IndexAffinityOk(pTerm.Expr, aff) ) return 0;
  return 1;
}
#endif

//	If the query plan for pSrc specified in pCost is a full table scan and indexing is allows (if there is no NOT INDEXED clause) and it possible to construct a transient index that would perform better than a full table scan even when the cost of constructing the index is taken into account, then alter the query plan to use the transient index.
static void bestAutomaticIndex(
  Parse *pParse,              /* The parsing context */
  WhereClause *pWC,           /* The WHERE clause */
  struct SrcList_item *pSrc,  /* The FROM clause term to search */
  Bitmask notReady,           /* Mask of cursors that are not available */
  WhereCost *pCost            /* Lowest cost query plan */
){
  double nTableRow;           /* Rows in the input table */
  double logN;                /* log(nTableRow) */
  double costTempIdx;         /* per-query cost of the transient index */
  WhereTerm *pTerm;           /* A single term of the WHERE clause */
  WhereTerm *pWCEnd;          /* End of pWC.Terms[] */
  Table *pTable;              /* Table tht might be indexed */

  if( pParse.nQueryLoop<=(double)1 ){
    /* There is no point in building an automatic index for a single scan */
    return;
  }
  if( (pParse.db.flags & SQLITE_AutoIndex)==0 ){
    /* Automatic indices are disabled at run-time */
    return;
  }
  if( (pCost.plan.wsFlags & WHERE_NOT_FULLSCAN)!=0 ){
    /* We already have some kind of index in use for this query. */
    return;
  }
  if( pSrc.notIndexed ){
    /* The NOT INDEXED clause appears in the SQL. */
    return;
  }
  if( pSrc.isCorrelated ){
    /* The source is a correlated sub-query. No point in indexing it. */
    return;
  }

  assert( pParse.nQueryLoop >= (double)1 );
  pTable = pSrc.pTab;
  nTableRow = pTable.nRowEst;
  logN = estLog(nTableRow);
  costTempIdx = 2*logN*(nTableRow/pParse.nQueryLoop + 1);
  if( costTempIdx>=pCost.rCost ){
    /* The cost of creating the transient table would be greater than
    ** doing the full table scan */
    return;
  }

  /* Search for any equality comparison term */
  pWCEnd = &pWC.Terms[len(pWC.Terms)]
  for(pTerm=pWC.Terms; pTerm<pWCEnd; pTerm++){
    if( termCanDriveIndex(pTerm, pSrc, notReady) ){
      WHERETRACE( "auto-index reduces cost from %.1f to %.1f\n", pCost.rCost, costTempIdx )
      pCost.rCost = costTempIdx;
      pCost.plan.nRow = logN + 1;
      pCost.plan.wsFlags = WHERE_TEMP_INDEX;
      pCost.used = pTerm.prereqRight;
      break;
    }
  }
}

//	Generate code to construct the Index object for an automatic index and to set up the WhereLevel object pLevel so that the code generator makes use of the automatic index.
static void constructAutomaticIndex(
  Parse *pParse,              /* The parsing context */
  WhereClause *pWC,           /* The WHERE clause */
  struct SrcList_item *pSrc,  /* The FROM clause term to get the next index */
  Bitmask notReady,           /* Mask of cursors that are not available */
  WhereLevel *pLevel          /* Write new index here */
){
  int nColumn;                /* Number of columns in the constructed index */
  WhereTerm *pTerm;           /* A single term of the WHERE clause */
  WhereTerm *pWCEnd;          /* End of pWC.Terms[] */
  int nByte;                  /* Byte of memory needed for pIdx */
  Index *pIdx;                /* Object describing the transient index */
  Vdbe *v;                    /* Prepared statement under construction */
  int addrInit;               /* Address of the initialization bypass jump */
  Table *pTable;              /* The table being indexed */
  KeyInfo *pKeyinfo;          /* Key information for the index */   
  int addrTop;                /* Top of the index fill loop */
  int regRecord;              /* Register holding an index record */
  int n;                      /* Column counter */
  int i;                      /* Loop counter */
  int mxBitCol;               /* Maximum column in pSrc.colUsed */
  CollSeq *pColl;             /* Collating sequence to on a column */
  Bitmask idxCols;            /* Bitmap of columns used for indexing */
  Bitmask extraCols;          /* Bitmap of additional columns */

  /* Generate code to skip over the creation and initialization of the
  ** transient index on 2nd and subsequent iterations of the loop. */
  v = pParse.pVdbe;
  assert( v!=0 );
  addrInit = pParse.CodeOnce()

  /* Count the number of columns that will be added to the index
  ** and used to match WHERE clause constraints */
  nColumn = 0;
  pTable = pSrc.pTab;
  pWCEnd = &pWC.Terms[len(pWC.Terms)]
  idxCols = 0;
  for(pTerm=pWC.Terms; pTerm<pWCEnd; pTerm++){
    if( termCanDriveIndex(pTerm, pSrc, notReady) ){
      int iCol = pTerm.u.leftColumn;
      Bitmask cMask = iCol>=BMS ? ((Bitmask)1)<<(BMS-1) : ((Bitmask)1)<<iCol;
      if( (idxCols & cMask)==0 ){
        nColumn++;
        idxCols |= cMask;
      }
    }
  }
  assert( nColumn>0 );
  pLevel.plan.nEq = nColumn;

  /* Count the number of additional columns needed to create a
  ** covering index.  A "covering index" is an index that contains all
  ** columns that are needed by the query.  With a covering index, the
  ** original table never needs to be accessed.  Automatic indices must
  ** be a covering index because the index will not be updated if the
  ** original table changes and the index and table cannot both be used
  ** if they go out of sync.
  */
  extraCols = pSrc.colUsed & (~idxCols | (((Bitmask)1)<<(BMS-1)));
  mxBitCol = (pTable.nCol >= BMS-1) ? BMS-1 : pTable.nCol;
  for(i=0; i<mxBitCol; i++){
    if( extraCols & (((Bitmask)1)<<i) ) nColumn++;
  }
  if( pSrc.colUsed & (((Bitmask)1)<<(BMS-1)) ){
    nColumn += pTable.nCol - BMS + 1;
  }
  pLevel.plan.wsFlags |= WHERE_COLUMN_EQ | WHERE_IDX_ONLY | WO_EQ;

  /* Construct the Index object to describe this index */
  nByte = sizeof(Index);
  nByte += nColumn*sizeof(int);     /* Index.Columns */
  nByte += nColumn*sizeof(char*);   /* Index.azColl */
  nByte += nColumn;                 /* Index.aSortOrder */
  pIdx = sqlite3DbMallocZero(pParse.db, nByte);
  if( pIdx==0 ) return;
  pLevel.plan.u.pIdx = pIdx;
  pIdx.azColl = (char**)&pIdx[1];
  pIdx.Columns = (int*)&pIdx.azColl[nColumn];
  pIdx.aSortOrder = (byte*)&pIdx.Columns[nColumn];
  pIdx.Name = "auto-index";
//  pIdx.nColumn = nColumn;
  pIdx.pTable = pTable;
  n = 0;
  idxCols = 0;
  for(pTerm=pWC.Terms; pTerm<pWCEnd; pTerm++){
    if( termCanDriveIndex(pTerm, pSrc, notReady) ){
      int iCol = pTerm.u.leftColumn;
      Bitmask cMask = iCol>=BMS ? ((Bitmask)1)<<(BMS-1) : ((Bitmask)1)<<iCol;
      if( (idxCols & cMask)==0 ){
        Expr *pX = pTerm.Expr;
        idxCols |= cMask;
        pIdx.Columns[n] = pTerm.u.leftColumn;
        pColl = sqlite3BinaryCompareCollSeq(pParse, pX.pLeft, pX.pRight);
        pIdx.azColl[n] = pColl ? pColl.Name : "BINARY";
        n++;
      }
    }
  }
  assert( (uint32)n==pLevel.plan.nEq );

  /* Add additional columns needed to make the automatic index into
  ** a covering index */
  for(i=0; i<mxBitCol; i++){
    if( extraCols & (((Bitmask)1)<<i) ){
      pIdx.Columns[n] = i;
      pIdx.azColl[n] = "BINARY";
      n++;
    }
  }
  if( pSrc.colUsed & (((Bitmask)1)<<(BMS-1)) ){
    for(i=BMS-1; i<pTable.nCol; i++){
      pIdx.Columns[n] = i;
      pIdx.azColl[n] = "BINARY";
      n++;
    }
  }
  assert( n==nColumn );

  /* Create the automatic index */
  pKeyinfo = pParse.IndexKeyinfo(pIdx)
  assert( pLevel.iIdxCur>=0 );
  sqlite3VdbeAddOp4(v, OP_OpenAutoindex, pLevel.iIdxCur, nColumn+1, 0,
                    (char*)pKeyinfo, P4_KEYINFO_HANDOFF);
  v.Comment("for ", pTable.Name)

  /* Fill the automatic index with content */
  addrTop = v.AddOp1(OP_Rewind, pLevel.iTabCur);
  regRecord = pParse.GetTempReg()
  sqlite3GenerateIndexKey(pParse, pIdx, pLevel.iTabCur, regRecord, 1);
  v.AddOp2(OP_IdxInsert, pLevel.iIdxCur, regRecord);
  v.ChangeP5(OPFLAG_USESEEKRESULT)
  v.AddOp2(OP_Next, pLevel.iTabCur, addrTop+1);
  v.ChangeP5(SQLITE_STMTSTATUS_AUTOINDEX)
  v.JumpHere(addrTop)
  pParse.ReleaseTempReg(regRecord)
  
  /* Jump here when skipping the initialization */
  v.JumpHere(addrInit)
}

/*
** Allocate and populate an sqlite3_index_info structure.
*/
static sqlite3_index_info *allocateIndexInfo(
  Parse *pParse, 
  WhereClause *pWC,
  struct SrcList_item *pSrc,
  ExprList *pOrderBy
){
  int i, j;
  int nTerm;
  struct sqlite3_index_constraint *pIdxCons;
  struct sqlite3_index_orderby *pIdxOrderBy;
  struct sqlite3_index_constraint_usage *pUsage;
  WhereTerm *pTerm;
  int nOrderBy;

  WHERETRACE( "Recomputing index info for %s...\n", pSrc.pTab.Name )

  /* Count the number of possible WHERE clause constraints referring
  ** to this virtual table */
  for(i=nTerm=0, pTerm=pWC.Terms; i<len(pWC.Terms); i++, pTerm++){
    if( pTerm.leftCursor != pSrc.iCursor ) continue;
    assert( (pTerm.eOperator&(pTerm.eOperator-1))==0 );
    if( pTerm.eOperator & (WO_IN|WO_ISNULL) ) continue;
    if( pTerm.wtFlags & TERM_VNULL ) continue;
    nTerm++;
  }

  /* If the ORDER BY clause contains only columns in the current 
  ** virtual table then allocate space for the aOrderBy part of
  ** the sqlite3_index_info structure.
  */
  nOrderBy = 0;
  if( pOrderBy ){
    for(i=0; i<pOrderBy.nExpr; i++){
      Expr *pExpr = pOrderBy.a[i].Expr;
      if( pExpr.op!=TK_COLUMN || pExpr.iTable!=pSrc.iCursor ) break;
    }
    if( i==pOrderBy.nExpr ){
      nOrderBy = pOrderBy.nExpr;
    }
  }

  pIdxInfo := new(sqlite3_index_info)

  /* Initialize the structure.  The sqlite3_index_info structure contains
  ** many fields that are declared "const" to prevent xBestIndex from
  ** changing them.  We have to do some funky casting in order to
  ** initialize those fields.
  */
  pIdxCons = (struct sqlite3_index_constraint*)&pIdxInfo[1];
  pIdxOrderBy = (struct sqlite3_index_orderby*)&pIdxCons[nTerm];
  pUsage = (struct sqlite3_index_constraint_usage*)&pIdxOrderBy[nOrderBy];
  *(int*)&pIdxInfo.nConstraint = nTerm;
  *(int*)&pIdxInfo.nOrderBy = nOrderBy;
  *(struct sqlite3_index_constraint**)&pIdxInfo.aConstraint = pIdxCons;
  *(struct sqlite3_index_orderby**)&pIdxInfo.aOrderBy = pIdxOrderBy;
  *(struct sqlite3_index_constraint_usage**)&pIdxInfo.aConstraintUsage =
                                                                   pUsage;

  for(i=j=0, pTerm=pWC.Terms; i<len(pWC.Terms); i++, pTerm++){
    if( pTerm.leftCursor != pSrc.iCursor ) continue;
    assert( (pTerm.eOperator&(pTerm.eOperator-1))==0 );
    if( pTerm.eOperator & (WO_IN|WO_ISNULL) ) continue;
    if( pTerm.wtFlags & TERM_VNULL ) continue;
    pIdxCons[j].iColumn = pTerm.u.leftColumn;
    pIdxCons[j].iTermOffset = i;
    pIdxCons[j].op = (byte)pTerm.eOperator;
    /* The direct assignment in the previous line is possible only because
    ** the WO_ and SQLITE_INDEX_CONSTRAINT_ codes are identical.  The
    ** following asserts verify this fact. */
    assert( WO_EQ==SQLITE_INDEX_CONSTRAINT_EQ );
    assert( WO_LT==SQLITE_INDEX_CONSTRAINT_LT );
    assert( WO_LE==SQLITE_INDEX_CONSTRAINT_LE );
    assert( WO_GT==SQLITE_INDEX_CONSTRAINT_GT );
    assert( WO_GE==SQLITE_INDEX_CONSTRAINT_GE );
    assert( WO_MATCH==SQLITE_INDEX_CONSTRAINT_MATCH );
    assert( pTerm.eOperator & (WO_EQ|WO_LT|WO_LE|WO_GT|WO_GE|WO_MATCH) );
    j++;
  }
  for(i=0; i<nOrderBy; i++){
    Expr *pExpr = pOrderBy.a[i].Expr;
    pIdxOrderBy[i].iColumn = pExpr.iColumn;
    pIdxOrderBy[i].desc = pOrderBy.a[i].sortOrder;
  }

  return pIdxInfo;
}

/*
** The table object reference passed as the second argument to this function
** must represent a virtual table. This function invokes the xBestIndex()
** method of the virtual table with the sqlite3_index_info pointer passed
** as the argument.
**
** If an error occurs, pParse is populated with an error message and a
** non-zero value is returned. Otherwise, 0 is returned and the output
** part of the sqlite3_index_info structure is left populated.
**
** Whether or not an error is returned, it is the responsibility of the
** caller to eventually free p.idxStr if p.needToFreeIdxStr indicates
** that this is required.
*/
static int vtabBestIndex(Parse *pParse, Table *pTab, sqlite3_index_info *p){
  sqlite3_vtab *pVtab = sqlite3GetVTable(pParse.db, pTab).pVtab;
  int i;
  int rc;

  WHERETRACE( "xBestIndex for %s\n", pTab.Name )
  TRACE_IDX_INPUTS(p);
  rc = pVtab.Callbacks.xBestIndex(pVtab, p);
  TRACE_IDX_OUTPUTS(p);

  if( rc!=SQLITE_OK ){
    if( rc==SQLITE_NOMEM ){
      pParse.db.mallocFailed = true
    }else if( !pVtab.zErrMsg ){
      pParse.SetErrorMsg("%v", sqlite3ErrStr(rc));
    }else{
      pParse.SetErrorMsg("%v", pVtab.zErrMsg);
    }
  }
  pVtab.zErrMsg = nil

  for(i=0; i<p.nConstraint; i++){
    if( !p.aConstraint[i].usable && p.aConstraintUsage[i].argvIndex>0 ){
      pParse, "table %v: xBestIndex returned an invalid plan", pTab.Name);
    }
  }

  return pParse.nErr;
}


/*
** Compute the best index for a virtual table.
**
** The best index is computed by the xBestIndex method of the virtual
** table module.  This routine is really just a wrapper that sets up
** the sqlite3_index_info structure that is used to communicate with
** xBestIndex.
**
** In a join, this routine might be called multiple times for the
** same virtual table.  The sqlite3_index_info structure is created
** and initialized on the first invocation and reused on all subsequent
** invocations.  The sqlite3_index_info structure is also used when
** code is generated to access the virtual table.  The whereInfoDelete() 
** routine takes care of freeing the sqlite3_index_info structure after
** everybody has finished with it.
*/
static void bestVirtualIndex(
  Parse *pParse,                  /* The parsing context */
  WhereClause *pWC,               /* The WHERE clause */
  struct SrcList_item *pSrc,      /* The FROM clause term to search */
  Bitmask notReady,               /* Mask of cursors not available for index */
  Bitmask notValid,               /* Cursors not valid for any purpose */
  ExprList *pOrderBy,             /* The order by clause */
  WhereCost *pCost,               /* Lowest cost query plan */
  sqlite3_index_info **ppIdxInfo  /* Index information passed to xBestIndex */
){
  Table *pTab = pSrc.pTab;
  sqlite3_index_info *pIdxInfo;
  struct sqlite3_index_constraint *pIdxCons;
  struct sqlite3_index_constraint_usage *pUsage;
  WhereTerm *pTerm;
  int i, j;
  int nOrderBy;
  double rCost;

  /* Make sure wsFlags is initialized to some sane value. Otherwise, if the 
  ** malloc in allocateIndexInfo() fails and this function returns leaving
  ** wsFlags in an uninitialized state, the caller may behave unpredictably.
  */
  memset(pCost, 0, sizeof(*pCost));
  pCost.plan.wsFlags = WHERE_VIRTUALTABLE;

  /* If the sqlite3_index_info structure has not been previously
  ** allocated and initialized, then allocate and initialize it now.
  */
  pIdxInfo = *ppIdxInfo;
  if( pIdxInfo==0 ){
    *ppIdxInfo = pIdxInfo = allocateIndexInfo(pParse, pWC, pSrc, pOrderBy);
  }
  if( pIdxInfo==0 ){
    return;
  }

  /* At this point, the sqlite3_index_info structure that pIdxInfo points
  ** to will have been initialized, either during the current invocation or
  ** during some prior invocation.  Now we just have to customize the
  ** details of pIdxInfo for the current invocation and pass it to
  ** xBestIndex.
  */

  /* The module name must be defined. Also, by this point there must
  ** be a pointer to an sqlite3_vtab structure. Otherwise
  ** sqlite3ViewGetColumnNames() would have picked up the error. 
  */
  assert( pTab.azModuleArg && pTab.azModuleArg[0] );
  assert( sqlite3GetVTable(pParse.db, pTab) );

  /* Set the aConstraint[].usable fields and initialize all 
  ** output variables to zero.
  **
  ** aConstraint[].usable is true for constraints where the right-hand
  ** side contains only references to tables to the left of the current
  ** table.  In other words, if the constraint is of the form:
  **
  **           column = expr
  **
  ** and we are evaluating a join, then the constraint on column is 
  ** only valid if all tables referenced in expr occur to the left
  ** of the table containing column.
  **
  ** The aConstraints[] array contains entries for all constraints
  ** on the current table.  That way we only have to compute it once
  ** even though we might try to pick the best index multiple times.
  ** For each attempt at picking an index, the order of tables in the
  ** join might be different so we have to recompute the usable flag
  ** each time.
  */
  pIdxCons = *(struct sqlite3_index_constraint**)&pIdxInfo.aConstraint;
  pUsage = pIdxInfo.aConstraintUsage;
  for(i=0; i<pIdxInfo.nConstraint; i++, pIdxCons++){
    j = pIdxCons.iTermOffset;
    pTerm = &pWC.Terms[j];
    pIdxCons.usable = (pTerm.prereqRight&notReady) ? 0 : 1;
  }
  memset(pUsage, 0, sizeof(pUsage[0])*pIdxInfo.nConstraint);
  if( pIdxInfo.needToFreeIdxStr ){
    pIdxInfo.idxStr = nil
  }
  pIdxInfo.idxStr = 0;
  pIdxInfo.idxNum = 0;
  pIdxInfo.needToFreeIdxStr = 0;
  pIdxInfo.orderByConsumed = 0;
  pIdxInfo.estimatedCost = BIG_DOUBLE / ((double)2);
  nOrderBy = pIdxInfo.nOrderBy;
  if( !pOrderBy ){
    pIdxInfo.nOrderBy = 0;
  }

  if( vtabBestIndex(pParse, pTab, pIdxInfo) ){
    return;
  }

  pIdxCons = *(struct sqlite3_index_constraint**)&pIdxInfo.aConstraint;
  for(i=0; i<pIdxInfo.nConstraint; i++){
    if( pUsage[i].argvIndex>0 ){
      pCost.used |= pWC.Terms[pIdxCons[i].iTermOffset].prereqRight;
    }
  }

  /* If there is an ORDER BY clause, and the selected virtual table index
  ** does not satisfy it, increase the cost of the scan accordingly. This
  ** matches the processing for non-virtual tables in bestBtreeIndex().
  */
  rCost = pIdxInfo.estimatedCost;
  if( pOrderBy && pIdxInfo.orderByConsumed==0 ){
    rCost += estLog(rCost)*rCost;
  }

  /* The cost is not allowed to be larger than BIG_DOUBLE (the
  ** inital value of lowestCost in this loop. If it is, then the
  ** (cost<lowestCost) test below will never be true.
  ** 
  ** Use "(double)2" instead of "2.0" in case OMIT_FLOATING_POINT 
  ** is defined.
  */
  if( (BIG_DOUBLE/((double)2))<rCost ){
    pCost.rCost = (BIG_DOUBLE/((double)2));
  }else{
    pCost.rCost = rCost;
  }
  pCost.plan.u.pVtabIdx = pIdxInfo;
  if( pIdxInfo.orderByConsumed ){
    pCost.plan.wsFlags |= WHERE_ORDERBY;
  }
  pCost.plan.nEq = 0;
  pIdxInfo.nOrderBy = nOrderBy;

  /* Try to find a more efficient access pattern by using multiple indexes
  ** to optimize an OR expression within the WHERE clause. 
  */
  bestOrClauseIndex(pParse, pWC, pSrc, notReady, notValid, pOrderBy, pCost);
}

/*
** Estimate the location of a particular key among all keys in an
** index.  Store the results in aStat as follows:
**
**    aStat[0]      Est. number of rows less than pVal
**    aStat[1]      Est. number of rows equal to pVal
**
** Return SQLITE_OK on success.
*/
static int whereKeyStats(
  Parse *pParse,              /* Database connection */
  Index *pIdx,                /* Index to consider domain of */
  sqlite3_value *pVal,        /* Value to consider */
  int roundUp,                /* Round up if true.  Round down if false */
  tRowcnt *aStat              /* OUT: stats written here */
){
  tRowcnt n;
  IndexSample *aSample;
  int i, eType;
  int isEq = 0;
  int64 v;
  double r, rS;

  assert( roundUp==0 || roundUp==1 );
  assert( pIdx.nSample>0 );
  if( pVal==0 ) return SQLITE_ERROR;
  n = pIdx.aiRowEst[0];
  aSample = pIdx.aSample;
  eType = sqlite3_value_type(pVal);

  if( eType==SQLITE_INTEGER ){
    v = sqlite3_value_int64(pVal);
    r = (int64)v;
    for(i=0; i<pIdx.nSample; i++){
      if( aSample[i].eType==SQLITE_NULL ) continue;
      if( aSample[i].eType>=SQLITE_TEXT ) break;
      if( aSample[i].eType==SQLITE_INTEGER ){
        if( aSample[i].u.i>=v ){
          isEq = aSample[i].u.i==v;
          break;
        }
      }else{
        assert( aSample[i].eType==SQLITE_FLOAT );
        if( aSample[i].u.r>=r ){
          isEq = aSample[i].u.r==r;
          break;
        }
      }
    }
  }else if( eType==SQLITE_FLOAT ){
    r = sqlite3_value_double(pVal);
    for(i=0; i<pIdx.nSample; i++){
      if( aSample[i].eType==SQLITE_NULL ) continue;
      if( aSample[i].eType>=SQLITE_TEXT ) break;
      if( aSample[i].eType==SQLITE_FLOAT ){
        rS = aSample[i].u.r;
      }else{
        rS = aSample[i].u.i;
      }
      if( rS>=r ){
        isEq = rS==r;
        break;
      }
    }
  }else if( eType==SQLITE_NULL ){
    i = 0;
    if( aSample[0].eType==SQLITE_NULL ) isEq = 1;
  }else{
    assert( eType==SQLITE_TEXT || eType==SQLITE_BLOB );
    for(i=0; i<pIdx.nSample; i++){
      if( aSample[i].eType==SQLITE_TEXT || aSample[i].eType==SQLITE_BLOB ){
        break;
      }
    }
    if( i<pIdx.nSample ){      
      sqlite3 *db = pParse.db;
      CollSeq *pColl;
      const byte *z;
      if( eType==SQLITE_BLOB ){
        z = (const byte *)sqlite3_value_blob(pVal);
        pColl = db.pDfltColl;
        assert( pColl.enc==SQLITE_UTF8 );
      }else{
        pColl = sqlite3GetCollSeq(db, SQLITE_UTF8, 0, *pIdx.azColl);
        if( pColl==0 ){
          pParse.SetErrorMsg("no such collation sequence: %v", *pIdx.azColl);
          return SQLITE_ERROR;
        }
        z = (const byte *)sqlite3ValueText(pVal, pColl.enc);
        if( !z ){
          return SQLITE_NOMEM;
        }
        assert( z && pColl && pColl.xCmp );
      }
      n = pVal.ValueBytes(pColl.enc)
  
      for(; i<pIdx.nSample; i++){
        int c;
        int eSampletype = aSample[i].eType;
        if( eSampletype<eType ) continue;
        if( eSampletype!=eType ) break;
        {
          c = pColl.xCmp(pColl.pUser, aSample[i].nByte, aSample[i].u.z, n, z);
        }
        if( c>=0 ){
          if( c==0 ) isEq = 1;
          break;
        }
      }
    }
  }

  /* At this point, aSample[i] is the first sample that is greater than
  ** or equal to pVal.  Or if i==pIdx.nSample, then all samples are less
  ** than pVal.  If aSample[i]==pVal, then isEq==1.
  */
  if( isEq ){
    assert( i<pIdx.nSample );
    aStat[0] = aSample[i].nLt;
    aStat[1] = aSample[i].nEq;
  }else{
    tRowcnt iLower, iUpper, iGap;
    if( i==0 ){
      iLower = 0;
      iUpper = aSample[0].nLt;
    }else{
      iUpper = i>=pIdx.nSample ? n : aSample[i].nLt;
      iLower = aSample[i-1].nEq + aSample[i-1].nLt;
    }
    aStat[1] = pIdx.avgEq;
    if( iLower>=iUpper ){
      iGap = 0;
    }else{
      iGap = iUpper - iLower;
    }
    if( roundUp ){
      iGap = (iGap*2)/3;
    }else{
      iGap = iGap/3;
    }
    aStat[0] = iLower + iGap;
  }
  return SQLITE_OK;
}

/*
** If expression pExpr represents a literal value, set *pp to point to
** an sqlite3_value structure containing the same value, with affinity
** aff applied to it, before returning. It is the responsibility of the 
** caller to eventually release this structure by passing it to 
** Mem::Free().
**
** If the current parse is a recompile (sqlite3Reprepare()) and pExpr
** is an SQL variable that currently has a non-NULL value bound to it,
** create an sqlite3_value structure containing this value, again with
** affinity aff applied to it, instead.
**
** If neither of the above apply, set *pp to NULL.
**
** If an error occurs, return an error code. Otherwise, SQLITE_OK.
*/
func (pParse *Parse) valueFromExpr(pExpr *Expr, aff byte) (pp *sqlite_value, rc int) {
	if pExpr.op == TK_VARIABLE || (pExpr.op == TK_REGISTER && pExpr.op2 == TK_VARIABLE) {
		iVar := pExpr.iColumn
		sqlite3VdbeSetVarmask(pParse.pVdbe, iVar)
		return sqlite3VdbeGetValue(pParse.pReprepare, iVar, aff), SQLITE_OK
	}
	return pParse.db.ValueFromExpr(pExpr, SQLITE_UTF8, aff)
}

/*
** This function is used to estimate the number of rows that will be visited
** by scanning an index for a range of values. The range may have an upper
** bound, a lower bound, or both. The WHERE clause terms that set the upper
** and lower bounds are represented by pLower and pUpper respectively. For
** example, assuming that index p is on t1(a):
**
**   ... FROM t1 WHERE a > ? AND a < ? ...
**                    |_____|   |_____|
**                       |         |
**                     pLower    pUpper
**
** If either of the upper or lower bound is not present, then NULL is passed in
** place of the corresponding WhereTerm.
**
** The nEq parameter is passed the index of the index column subject to the
** range constraint. Or, equivalently, the number of equality constraints
** optimized by the proposed index scan. For example, assuming index p is
** on t1(a, b), and the SQL query is:
**
**   ... FROM t1 WHERE a = ? AND b > ? AND b < ? ...
**
** then nEq should be passed the value 1 (as the range restricted column,
** b, is the second left-most column of the index). Or, if the query is:
**
**   ... FROM t1 WHERE a > ? AND a < ? ...
**
** then nEq should be passed 0.
**
** The returned value is an integer divisor to reduce the estimated
** search space.  A return value of 1 means that range constraints are
** no help at all.  A return value of 2 means range constraints are
** expected to reduce the search space by half.  And so forth...
**
** In the absence of sqlite_stat3 ANALYZE data, each range inequality
** reduces the search space by a factor of 4.  Hence a single constraint (x>?)
** results in a return of 4 and a range constraint (x>? AND x<?) results
** in a return of 16.
*/
static int whereRangeScanEst(
  Parse *pParse,       /* Parsing & code generating context */
  Index *p,            /* The index containing the range-compared column; "x" */
  int nEq,             /* index into p.Columns[] of the range-compared column */
  WhereTerm *pLower,   /* Lower bound on the range. ex: "x>123" Might be NULL */
  WhereTerm *pUpper,   /* Upper bound on the range. ex: "x<455" Might be NULL */
  double *pRangeDiv   /* OUT: Reduce search space by this divisor */
){
  int rc = SQLITE_OK;

  if( nEq==0 && p.nSample ){
    sqlite3_value *pRangeVal;
    tRowcnt iLower = 0;
    tRowcnt iUpper = p.aiRowEst[0];
    tRowcnt a[2];
    byte aff = p.pTable.Columns[p.Columns[0]].affinity;

    if( pLower ){
      Expr *pExpr = pLower.Expr.pRight;
      pRangeVal, rc = pParse.valueFromExpr(pExpr, aff)
      assert( pLower.eOperator==WO_GT || pLower.eOperator==WO_GE );
      if( rc==SQLITE_OK
       && whereKeyStats(pParse, p, pRangeVal, 0, a)==SQLITE_OK
      ){
        iLower = a[0];
        if( pLower.eOperator==WO_GT ) iLower += a[1];
      }
      pRangeVal.Free()
    }
    if( rc==SQLITE_OK && pUpper ){
      Expr *pExpr = pUpper.Expr.pRight;
      pRangeVal, rc = pParse.valueFromExpr(pExpr, aff)
      assert( pUpper.eOperator==WO_LT || pUpper.eOperator==WO_LE );
      if( rc==SQLITE_OK
       && whereKeyStats(pParse, p, pRangeVal, 1, a)==SQLITE_OK
      ){
        iUpper = a[0];
        if( pUpper.eOperator==WO_LE ) iUpper += a[1];
      }
      pRangeVal.Free()
    }
    if( rc==SQLITE_OK ){
      if( iUpper<=iLower ){
        *pRangeDiv = (double)p.aiRowEst[0];
      }else{
        *pRangeDiv = (double)p.aiRowEst[0]/(double)(iUpper - iLower);
      }
      WHERETRACE( "range scan regions: %u..%u  div=%g\n", (uint32)iLower, (uint32)iUpper, *pRangeDiv )
      return SQLITE_OK;
    }
  }
  assert( pLower || pUpper );
  *pRangeDiv = (double)1;
  if( pLower && (pLower.wtFlags & TERM_VNULL)==0 ) *pRangeDiv *= (double)4;
  if( pUpper ) *pRangeDiv *= (double)4;
  return rc;
}

//	Estimate the number of rows that will be returned based on an equality constraint x=VALUE and where that VALUE occurs in the histogram data. This only works when x is the left-most column of an index and sqlite_stat3 histogram data is available for that index. When pExpr == NULL that means the constraint is "x IS NULL" instead of "x=VALUE".
//	Write the estimated row count into *pnRow and return SQLITE_OK. If unable to make an estimate, leave *pnRow unchanged and return non-zero.
//	This routine can fail if it is unable to load a collating sequence required for string comparison. The error is stored in the pParse structure.
func (pParse *Parse) whereEqualScanEst(index *Index, expression *Expr) (rows float32, rc int) {
	RHS		*sqlite3_value		//	VALUE on right-hand side of pTerm
	defer func() {
		RHS.Free()
	}()

	assert( len(index.aSample) != 0 )
	affinity := index.pTable.Columns[index.Columns[0]].affinity
	if expression != nil {
		if RHS, rc = pParse.valueFromExpr(expression, affinity); rc != SQLITE_OK {
			return
		}
	} else {
		RHS = pParse.db.NewValue()
	}
	stats := make(tRowcnt, 2)
	if RHS == nil {
		rc = SQLITE_NOTFOUND
	} else if rc = whereKeyStats(pParse, index, RHS, 0, stats); rc == SQLITE_OK {
		rows = stats[1]
		WHERETRACE( "equality scan regions: %v\n", int(rows) )
	}
}

/*
** Estimate the number of rows that will be returned based on
** an IN constraint where the right-hand side of the IN operator
** is a list of values.  Example:
**
**        WHERE x IN (1,2,3,4)
**
** Write the estimated row count into *pnRow and return SQLITE_OK. 
** If unable to make an estimate, leave *pnRow unchanged and return
** non-zero.
**
** This routine can fail if it is unable to load a collating sequence
** required for string comparison. The error is stored
** in the pParse structure.
*/
func (pParse *Parse) whereInScanEst(index *Index, expressions *ExprList) (rows float64, rc int) {
	nEst		float64			//	Number of rows for a single term
	nRowEst		float64			//	New estimate of the number of rows

	assert( len(index.aSample) != 0 )
	for _, item := range expressions.Items {
		if nEst, rc = pParse.whereEqualScanEst(index, item.Expr); rc == SQLITE_OK {
			nRowEst += nEst
		} else {
			nRowEst += index.aiRowEst[0]
			break
		}
	}
	if rc == SQLITE_OK {
		if nRowEst > index.aiRowEst[0] {
			nRowEst = index.aiRowEst[0]
		}
		rows = nRowEst
		WHERETRACE( "IN row estimate: est=%g\n", nRowEst )
	}
	return
}


/*
** Find the best query plan for accessing a particular table.  Write the
** best query plan and its cost into the WhereCost object supplied as the
** last parameter.
**
** The lowest cost plan wins.  The cost is an estimate of the amount of
** CPU and disk I/O needed to process the requested result.
** Factors that influence cost include:
**
**    *  The estimated number of rows that will be retrieved.  (The
**       fewer the better.)
**
**    *  Whether or not sorting must occur.
**
**    *  Whether or not there must be separate lookups in the
**       index and in the main table.
**
** If there was an INDEXED BY clause (pSrc.pIndex) attached to the table in
** the SQL statement, then this function only considers plans using the 
** named index. If no such plan is found, then the returned cost is
** BIG_DOUBLE. If a plan is found that uses the named index, 
** then the cost is calculated in the usual way.
**
** If a NOT INDEXED clause (pSrc.notIndexed!=0) was attached to the table 
** in the SELECT statement, then no indexes are considered. However, the 
** selected plan may still take advantage of the built-in rowid primary key
** index.
*/
static void bestBtreeIndex(
  Parse *pParse,              /* The parsing context */
  WhereClause *pWC,           /* The WHERE clause */
  struct SrcList_item *pSrc,  /* The FROM clause term to search */
  Bitmask notReady,           /* Mask of cursors not available for indexing */
  Bitmask notValid,           /* Cursors not available for any purpose */
  ExprList *pOrderBy,         /* The ORDER BY clause */
  ExprList *pDistinct,        /* The select-list if query is DISTINCT */
  WhereCost *pCost            /* Lowest cost query plan */
){
  int iCur = pSrc.iCursor;   /* The cursor of the table to be accessed */
  Index *pProbe;              /* An index we are evaluating */
  Index *pIdx;                /* Copy of pProbe, or zero for IPK index */
  int eqTermMask;             /* Current mask of valid equality operators */
  int idxEqTermMask;          /* Index mask of valid equality operators */
  Index sPk;                  /* A fake index object for the primary key */
  tRowcnt aiRowEstPk[2];      /* The aiRowEst[] value for the sPk index */
  ColumnsPk := -1;        /* The aColumn[] value for the sPk index */
  int wsFlagMask;             /* Allowed flags in pCost.plan.wsFlag */

  /* Initialize the cost to a worst-case value */
  memset(pCost, 0, sizeof(*pCost));
  pCost.rCost = BIG_DOUBLE;

  /* If the pSrc table is the right table of a LEFT JOIN then we may not
  ** use an index to satisfy IS NULL constraints on that table.  This is
  ** because columns might end up being NULL if the table does not match -
  ** a circumstance which the index cannot help us discover.  Ticket #2177.
  */
  if( pSrc.jointype & JT_LEFT ){
    idxEqTermMask = WO_EQ|WO_IN;
  }else{
    idxEqTermMask = WO_EQ|WO_IN|WO_ISNULL;
  }

  if( pSrc.pIndex ){
    /* An INDEXED BY clause specifies a particular index to use */
    pIdx = pProbe = pSrc.pIndex;
    wsFlagMask = ~(WHERE_ROWID_EQ|WHERE_ROWID_RANGE);
    eqTermMask = idxEqTermMask;
  }else{
    /* There is no INDEXED BY clause.  Create a fake Index object in local
    ** variable sPk to represent the rowid primary key index.  Make this
    ** fake index the first in a chain of Index objects with all of the real
    ** indices to follow */
    memset(&sPk, 0, sizeof(Index));
//    sPk.nColumn = 1;
    sPk.Columns = &ColumnsPk;
    sPk.aiRowEst = aiRowEstPk;
    sPk.onError = OE_Replace;
    sPk.pTable = pSrc.pTab;
    aiRowEstPk[0] = pSrc.pTab.nRowEst;
    aiRowEstPk[1] = 1;
    pFirst := pSrc.pTab.Indices[0]
    if !pSrc.notIndexed {
      /* The real indices of the table are only considered if the
      ** NOT INDEXED qualifier is omitted from the FROM clause */
      sPk.Next = pFirst;
    }
    pProbe = &sPk;
    wsFlagMask = ~(
        WHERE_COLUMN_IN|WHERE_COLUMN_EQ|WHERE_COLUMN_NULL|WHERE_COLUMN_RANGE
    );
    eqTermMask = WO_EQ|WO_IN;
    pIdx = 0;
  }

  /* Loop over all indices looking for the best one to use
  */
  for(; pProbe; pIdx=pProbe=pProbe.Next){
    const tRowcnt * const aiRowEst = pProbe.aiRowEst;
    double cost;                /* Cost of using pProbe */
    double log10N = (double)1;  /* base-10 logarithm of nRow (inexact) */
    int rev;                    /* True to scan in reverse order */
    int wsFlags = 0;
    Bitmask used = 0;

    /* The following variables are populated based on the properties of
    ** index being evaluated. They are then used to determine the expected
    ** cost and number of rows returned.
    **
    **  nEq: 
    **    Number of equality terms that can be implemented using the index.
    **    In other words, the number of initial fields in the index that
    **    are used in == or IN or NOT NULL constraints of the WHERE clause.
    **
    **  nInMul:  
    **    The "in-multiplier". This is an estimate of how many seek operations 
    **    SQLite must perform on the index in question. For example, if the 
    **    WHERE clause is:
    **
    **      WHERE a IN (1, 2, 3) AND b IN (4, 5, 6)
    **
    **    SQLite must perform 9 lookups on an index on (a, b), so nInMul is 
    **    set to 9. Given the same schema and either of the following WHERE 
    **    clauses:
    **
    **      WHERE a =  1
    **      WHERE a >= 2
    **
    **    nInMul is set to 1.
    **
    **    If there exists a WHERE term of the form "x IN (SELECT ...)", then 
    **    the sub-select is assumed to return 25 rows for the purposes of 
    **    determining nInMul.
    **
    **  bInEst:  
    **    Set to true if there was at least one "x IN (SELECT ...)" term used 
    **    in determining the value of nInMul.  Note that the RHS of the
    **    IN operator must be a SELECT, not a value list, for this variable
    **    to be true.
    **
    **  rangeDiv:
    **    An estimate of a divisor by which to reduce the search space due
    **    to inequality constraints.  In the absence of sqlite_stat3 ANALYZE
    **    data, a single inequality reduces the search space to 1/4rd its
    **    original size (rangeDiv==4).  Two inequalities reduce the search
    **    space to 1/16th of its original size (rangeDiv==16).
    **
    **  bSort:   
    **    Boolean. True if there is an ORDER BY clause that will require an 
    **    external sort (i.e. scanning the index being evaluated will not 
    **    correctly order records).
    **
    **  bLookup: 
    **    Boolean. True if a table lookup is required for each index entry
    **    visited.  In other words, true if this is not a covering index.
    **    This is always false for the rowid primary key index of a table.
    **    For other indexes, it is true unless all the columns of the table
    **    used by the SELECT statement are present in the index (such an
    **    index is sometimes described as a covering index).
    **    For example, given the index on (a, b), the second of the following 
    **    two queries requires table b-tree lookups in order to find the value
    **    of column c, but the first does not because columns a and b are
    **    both available in the index.
    **
    **             SELECT a, b    FROM tbl WHERE a = 1;
    **             SELECT a, b, c FROM tbl WHERE a = 1;
    */
    int nEq;                      /* Number of == or IN terms matching index */
    int bInEst = 0;               /* True if "x IN (SELECT...)" seen */
    int nInMul = 1;               /* Number of distinct equalities to lookup */
    double rangeDiv = (double)1;  /* Estimated reduction in search space */
    int nBound = 0;               /* Number of range constraints seen */
    int bSort = !!pOrderBy;       /* True if external sort required */
    int bDist = !!pDistinct;      /* True if index cannot help with DISTINCT */
    int bLookup = 0;              /* True if not a covering index */
    WhereTerm *pTerm;             /* A single term of the WHERE clause */
    WhereTerm *pFirstTerm = 0;    /* First term matching the index */

    //	Determine the values of nEq and nInMul
	for nEq, j := range pProbe.Columns {
		if pTerm = findTerm(pWC, iCur, j, notReady, eqTermMask, pIdx); pTerm == 0 {
			break
		}
		wsFlags |= (WHERE_COLUMN_EQ|WHERE_ROWID_EQ)
		switch {
		case pTerm.eOperator & WO_IN:
			pExpr := pTerm.Expr
			wsFlags |= WHERE_COLUMN_IN
			switch {
			case pExpr.HasProperty(EP_xIsSelect):
				//	"x IN (SELECT ...)":  Assume the SELECT returns 25 rows
				nInMul *= 25
				bInEst = 1
			case pExpr.pList != nil && pExpr.pList.Len() != 0:
				//	"x IN (value, value, ...)"
				if n := pExpr.pList.Len(); n != 0 {
					nInMul *= n
				}
			}
		case pTerm.eOperator & WO_ISNULL:
			wsFlags |= WHERE_COLUMN_NULL
		}
		if nEq == 0 && pProbe.aSample {
			pFirstTerm = pTerm
		}
		used |= pTerm.prereqRight
	}
 
    /* If the index being considered is UNIQUE, and there is an equality 
    ** constraint for all columns in the index, then this search will find
    ** at most a single row. In this case set the WHERE_UNIQUE flag to 
    ** indicate this to the caller.
    **
    ** Otherwise, if the search may find more than one row, test to see if
    ** there is a range constraint on indexed column (nEq+1) that can be 
    ** optimized using the index. 
    */
    if( nEq==len(pProbe.Columns) && pProbe.onError!=OE_None ){
      if( (wsFlags & (WHERE_COLUMN_IN|WHERE_COLUMN_NULL))==0 ){
        wsFlags |= WHERE_UNIQUE;
      }
    }else if( pProbe.bUnordered==0 ){
      int j = (nEq == len(pProbe.Columns) ? -1 : pProbe.Columns[nEq]);
      if( findTerm(pWC, iCur, j, notReady, WO_LT|WO_LE|WO_GT|WO_GE, pIdx) ){
        WhereTerm *pTop = findTerm(pWC, iCur, j, notReady, WO_LT|WO_LE, pIdx);
        WhereTerm *pBtm = findTerm(pWC, iCur, j, notReady, WO_GT|WO_GE, pIdx);
        whereRangeScanEst(pParse, pProbe, nEq, pBtm, pTop, &rangeDiv);
        if( pTop ){
          nBound = 1;
          wsFlags |= WHERE_TOP_LIMIT;
          used |= pTop.prereqRight;
        }
        if( pBtm ){
          nBound++;
          wsFlags |= WHERE_BTM_LIMIT;
          used |= pBtm.prereqRight;
        }
        wsFlags |= (WHERE_COLUMN_RANGE|WHERE_ROWID_RANGE);
      }
    }

    /* If there is an ORDER BY clause and the index being considered will
    ** naturally scan rows in the required order, set the appropriate flags
    ** in wsFlags. Otherwise, if there is an ORDER BY clause but the index
    ** will scan rows in a different order, set the bSort variable.  */
    if( isSortingIndex(
          pParse, pWC.WhereMaskSet, pProbe, iCur, pOrderBy, nEq, wsFlags, &rev)
    ){
      bSort = 0;
      wsFlags |= WHERE_ROWID_RANGE|WHERE_COLUMN_RANGE|WHERE_ORDERBY;
      wsFlags |= (rev ? WHERE_REVERSE : 0);
    }

    /* If there is a DISTINCT qualifier and this index will scan rows in
    ** order of the DISTINCT expressions, clear bDist and set the appropriate
    ** flags in wsFlags. */
    if( isDistinctIndex(pParse, pWC, pProbe, iCur, pDistinct, nEq)
     && (wsFlags & WHERE_COLUMN_IN)==0
    ){
      bDist = 0;
      wsFlags |= WHERE_ROWID_RANGE|WHERE_COLUMN_RANGE|WHERE_DISTINCT;
    }

    /* If currently calculating the cost of using an index (not the IPK
    ** index), determine if all required column data may be obtained without 
    ** using the main table (i.e. if the index is a covering
    ** index for this query). If it is, set the WHERE_IDX_ONLY flag in
    ** wsFlags. Otherwise, set the bLookup variable to true.  */
    if( pIdx && wsFlags ){
      Bitmask m = pSrc.colUsed;
      for _, column := range pIdx.Columns {
        if column < BMS - 1 {
          m &= ~(((Bitmask)1) << column)
        }
      }
      if( m==0 ){
        wsFlags |= WHERE_IDX_ONLY;
      }else{
        bLookup = 1;
      }
    }

	//	Estimate the number of rows of output.  For an "x IN (SELECT...)" constraint, do not let the estimate exceed half the rows in the table.
    nRow := float64(aiRowEst[nEq] * nInMul)
	if bInEst && nRow * 2 > aiRowEst[0] {
		nRow = aiRowEst[0] / 2
		nInMul = int(nRow / aiRowEst[nEq])
	}

	//	If the constraint is of the form x=VALUE or x IN (E1,E2,...) and we do not think that values of x are unique and if histogram data is available for column x, then it might be possible to get a better estimate on the number of rows based on VALUE and how common that value is according to the histogram.
	if nRow > 1 && nEq == 1 && pFirstTerm != 0 && aiRowEst[1] > 1 {
		assert( pFirstTerm.eOperator & (WO_EQ | WO_ISNULL | WO_IN) != 0 )
		switch {
		case pFirstTerm.eOperator & (WO_EQ | WO_ISNULL) != 0:
			if n, rc := pParse.whereEqualScanEst(pProbe, pFirstTerm.Expr.pRight) rc == SQLITE_OK {
				nRow = n
			}
		case !bInEst:
			assert( pFirstTerm.eOperator == WO_IN )
			if rows, rc := pParse.whereInScanEst(pProbe, pFirstTerm.Expr.pList); rc == SQLITE_OK {
				nRow = rows
			}
		}
	}

	//	Adjust the number of output rows and downward to reflect rows that are excluded by range constraints.
	if nRow = nRow / rangeDiv; nRow < 1 {
		nRow = 1
	}

    /* Experiments run on real SQLite databases show that the time needed
    ** to do a binary search to locate a row in a table or index is roughly
    ** log10(N) times the time to move from one row to the next row within
    ** a table or index.  The actual times can vary, with the size of
    ** records being an important factor.  Both moves and searches are
    ** slower with larger records, presumably because fewer records fit
    ** on one page and hence more pages have to be fetched.
    **
    ** The ANALYZE command and the sqlite_stat1 and sqlite_stat3 tables do
    ** not give us data on the relative sizes of table and index records.
    ** So this computation assumes table records are about twice as big
    ** as index records
    */
    if( (wsFlags & WHERE_NOT_FULLSCAN)==0 ){
      /* The cost of a full table scan is a number of move operations equal
      ** to the number of rows in the table.
      **
      ** We add an additional 4x penalty to full table scans.  This causes
      ** the cost function to err on the side of choosing an index over
      ** choosing a full scan.  This 4x full-scan penalty is an arguable
      ** decision and one which we expect to revisit in the future.  But
      ** it seems to be working well enough at the moment.
      */
      cost = aiRowEst[0]*4;
    }else{
      log10N = estLog(aiRowEst[0]);
      cost = nRow;
      if( pIdx ){
        if( bLookup ){
          /* For an index lookup followed by a table lookup:
          **    nInMul index searches to find the start of each index range
          **  + nRow steps through the index
          **  + nRow table searches to lookup the table entry using the rowid
          */
          cost += (nInMul + nRow)*log10N;
        }else{
          /* For a covering index:
          **     nInMul index searches to find the initial entry 
          **   + nRow steps through the index
          */
          cost += nInMul*log10N;
        }
      }else{
        /* For a rowid primary key lookup:
        **    nInMult table searches to find the initial entry for each range
        **  + nRow steps through the table
        */
        cost += nInMul*log10N;
      }
    }

    /* Add in the estimated cost of sorting the result.  Actual experimental
    ** measurements of sorting performance in SQLite show that sorting time
    ** adds C*N*log10(N) to the cost, where N is the number of rows to be 
    ** sorted and C is a factor between 1.95 and 4.3.  We will split the
    ** difference and select C of 3.0.
    */
    if( bSort ){
      cost += nRow*estLog(nRow)*3;
    }
    if( bDist ){
      cost += nRow*estLog(nRow)*3;
    }

    /**** Cost of using this index has now been computed ****/

    /* If there are additional constraints on this table that cannot
    ** be used with the current index, but which might lower the number
    ** of output rows, adjust the nRow value accordingly.  This only 
    ** matters if the current index is the least costly, so do not bother
    ** with this step if we already know this index will not be chosen.
    ** Also, never reduce the output row count below 2 using this step.
    **
    ** It is critical that the notValid mask be used here instead of
    ** the notReady mask.  When computing an "optimal" index, the notReady
    ** mask will only have one bit set - the bit for the current table.
    ** The notValid mask, on the other hand, always has all bits set for
    ** tables that are not in outer loops.  If notReady is used here instead
    ** of notValid, then a optimal index that depends on inner joins loops
    ** might be selected even when there exists an optimal index that has
    ** no such dependency.
    */
    if( nRow>2 && cost<=pCost.rCost ){
      int k;                       /* Loop counter */
      int nSkipEq = nEq;           /* Number of == constraints to skip */
      int nSkipRange = nBound;     /* Number of < constraints to skip */
      Bitmask thisTab;             /* Bitmap for pSrc */

      thisTab = getMask(pWC.WhereMaskSet, iCur);
      for(pTerm=pWC.Terms, k=len(pWC.Terms); nRow>2 && k; k--, pTerm++){
        if( pTerm.wtFlags & TERM_VIRTUAL ) continue;
        if( (pTerm.prereqAll & notValid)!=thisTab ) continue;
        if( pTerm.eOperator & (WO_EQ|WO_IN|WO_ISNULL) ){
          if( nSkipEq ){
            /* Ignore the first nEq equality matches since the index
            ** has already accounted for these */
            nSkipEq--;
          }else{
            /* Assume each additional equality match reduces the result
            ** set size by a factor of 10 */
            nRow /= 10;
          }
        }else if( pTerm.eOperator & (WO_LT|WO_LE|WO_GT|WO_GE) ){
          if( nSkipRange ){
            /* Ignore the first nSkipRange range constraints since the index
            ** has already accounted for these */
            nSkipRange--;
          }else{
            /* Assume each additional range constraint reduces the result
            ** set size by a factor of 3.  Indexed range constraints reduce
            ** the search space by a larger factor: 4.  We make indexed range
            ** more selective intentionally because of the subjective 
            ** observation that indexed range constraints really are more
            ** selective in practice, on average. */
            nRow /= 3;
          }
        }else if( pTerm.eOperator!=WO_NOOP ){
          /* Any other expression lowers the output row count by half */
          nRow /= 2;
        }
      }
      if( nRow<2 ) nRow = 2;
    }

    WHERETRACE( "%s(%s): nEq=%d nInMul=%d rangeDiv=%d bSort=%d bLookup=%d wsFlags=0x%x\n         notReady=0x%llx log10N=%.1f nRow=%.1f cost=%.1f used=0x%llx\n", pSrc.pTab.Name, (pIdx ? pIdx.Name : "ipk"), nEq, nInMul, (int)rangeDiv, bSort, bLookup, wsFlags, notReady, log10N, nRow, cost, used )

    /* If this index is the best we have seen so far, then record this
    ** index and its cost in the pCost structure.
    */
    if( (!pIdx || wsFlags)
     && (cost<pCost.rCost || (cost<=pCost.rCost && nRow<pCost.plan.nRow))
    ){
      pCost.rCost = cost;
      pCost.used = used;
      pCost.plan.nRow = nRow;
      pCost.plan.wsFlags = (wsFlags&wsFlagMask);
      pCost.plan.nEq = nEq;
      pCost.plan.u.pIdx = pIdx;
    }

    /* If there was an INDEXED BY clause, then only that one index is
    ** considered. */
    if( pSrc.pIndex ) break;

    /* Reset masks for the next index in the loop */
    wsFlagMask = ~(WHERE_ROWID_EQ|WHERE_ROWID_RANGE);
    eqTermMask = idxEqTermMask;
  }

  /* If there is no ORDER BY clause and the SQLITE_ReverseOrder flag
  ** is set, then reverse the order that the index will be scanned
  ** in. This is used for application testing, to help find cases
  ** where application behaviour depends on the (undefined) order that
  ** SQLite outputs rows in in the absence of an ORDER BY clause.  */
  if( !pOrderBy && pParse.db.flags & SQLITE_ReverseOrder ){
    pCost.plan.wsFlags |= WHERE_REVERSE;
  }

  assert( pOrderBy || (pCost.plan.wsFlags&WHERE_ORDERBY)==0 );
  assert( pCost.plan.u.pIdx==0 || (pCost.plan.wsFlags&WHERE_ROWID_EQ)==0 );
  assert( pSrc.pIndex==0 
       || pCost.plan.u.pIdx==0 
       || pCost.plan.u.pIdx==pSrc.pIndex 
  );

  WHERETRACE( "best index is: %s\n", (pCost.plan.wsFlags & WHERE_NOT_FULLSCAN) == 0 ? "none" : pCost.plan.u.pIdx ? pCost.plan.u.pIdx.Name : "ipk" )
  
  bestOrClauseIndex(pParse, pWC, pSrc, notReady, notValid, pOrderBy, pCost);
  bestAutomaticIndex(pParse, pWC, pSrc, notReady, pCost);
  pCost.plan.wsFlags |= eqTermMask;
}

/*
** Find the query plan for accessing table pSrc.pTab. Write the
** best query plan and its cost into the WhereCost object supplied 
** as the last parameter. This function may calculate the cost of
** both real and virtual table scans.
*/
static void bestIndex(
  Parse *pParse,              /* The parsing context */
  WhereClause *pWC,           /* The WHERE clause */
  struct SrcList_item *pSrc,  /* The FROM clause term to search */
  Bitmask notReady,           /* Mask of cursors not available for indexing */
  Bitmask notValid,           /* Cursors not available for any purpose */
  ExprList *pOrderBy,         /* The ORDER BY clause */
  WhereCost *pCost            /* Lowest cost query plan */
){
  if( pSrc.pTab.IsVirtual() ){
    sqlite3_index_info *p = 0;
    bestVirtualIndex(pParse, pWC, pSrc, notReady, notValid, pOrderBy, pCost,&p);
    if( p.needToFreeIdxStr ){
      p.idxStr = nil
    }
    p = nil
  } else {
    bestBtreeIndex(pParse, pWC, pSrc, notReady, notValid, pOrderBy, 0, pCost);
  }
}

/*
** Disable a term in the WHERE clause.  Except, do not disable the term
** if it controls a LEFT OUTER JOIN and it did not originate in the ON
** or USING clause of that join.
**
** Consider the term t2.z='ok' in the following queries:
**
**   (1)  SELECT * FROM t1 LEFT JOIN t2 ON t1.a=t2.x WHERE t2.z='ok'
**   (2)  SELECT * FROM t1 LEFT JOIN t2 ON t1.a=t2.x AND t2.z='ok'
**   (3)  SELECT * FROM t1, t2 WHERE t1.a=t2.x AND t2.z='ok'
**
** The t2.z='ok' is disabled in the in (2) because it originates
** in the ON clause.  The term is disabled in (3) because it is not part
** of a LEFT OUTER JOIN.  In (1), the term is not disabled.
**
** IMPLEMENTATION-OF: R-24597-58655 No tests are done for terms that are
** completely satisfied by indices.
**
** Disabling a term causes that term to not be tested in the inner loop
** of the join.  Disabling is an optimization.  When terms are satisfied
** by indices, we disable them to prevent redundant tests in the inner
** loop.  We would get the correct results if nothing were ever disabled,
** but joins might run a little slower.  The trick is to disable as much
** as we can without disabling too much.  If we disabled in (1), we'd get
** the wrong answer.  See ticket #813.
*/
static void disableTerm(WhereLevel *pLevel, WhereTerm *pTerm){
  if( pTerm
      && (pTerm.wtFlags & TERM_CODED)==0
      && (pLevel.iLeftJoin==0 || pTerm.Expr.HasProperty(EP_FromJoin))
  ){
    pTerm.wtFlags |= TERM_CODED;
    if( pTerm.iParent>=0 ){
      WhereTerm *pOther = &pTerm.pWC.Terms[pTerm.iParent];
      if( (--pOther.nChild)==0 ){
        disableTerm(pLevel, pOther);
      }
    }
  }
}

/*
** Code an OP_Affinity opcode to apply the column affinity string zAff
** to the n registers starting at base. 
**
** As an optimization, SQLITE_AFF_NONE entries (which are no-ops) at the
** beginning and end of zAff are ignored.  If all entries in zAff are
** SQLITE_AFF_NONE, then no code gets generated.
**
** This routine makes its own copy of zAff so that the caller is free
** to modify zAff after this routine returns.
*/
static void codeApplyAffinity(Parse *pParse, int base, int n, char *zAff){
  Vdbe *v = pParse.pVdbe;
  if( zAff==0 ){
    assert( pParse.db.mallocFailed );
    return;
  }
  assert( v!=0 );

  /* Adjust base and n to skip over SQLITE_AFF_NONE entries at the beginning
  ** and end of the affinity string.
  */
  while( n>0 && zAff[0]==SQLITE_AFF_NONE ){
    n--;
    base++;
    zAff++;
  }
  while( n>1 && zAff[n-1]==SQLITE_AFF_NONE ){
    n--;
  }

  /* Code the OP_Affinity opcode if there is anything left to do. */
  if( n>0 ){
    v.AddOp2(OP_Affinity, base, n);
    sqlite3VdbeChangeP4(v, -1, zAff, n);
    sqlite3ExprCacheAffinityChange(pParse, base, n);
  }
}


/*
** Generate code for a single equality term of the WHERE clause.  An equality
** term can be either X=expr or X IN (...).   pTerm is the term to be 
** coded.
**
** The current value for the constraint is left in register iReg.
**
** For a constraint of the form X=expr, the expression is evaluated and its
** result is left on the stack.  For constraints of the form X IN (...)
** this routine sets up a loop that will iterate over all values of X.
*/
static int codeEqualityTerm(
  Parse *pParse,      /* The parsing context */
  WhereTerm *pTerm,   /* The term of the WHERE clause to be coded */
  WhereLevel *pLevel, /* When level of the FROM clause we are working on */
  int iTarget         /* Attempt to leave results in this register */
){
  Expr *pX = pTerm.Expr;
  Vdbe *v = pParse.pVdbe;
  int iReg;                  /* Register holding results */

  assert( iTarget>0 );
  if( pX.op==TK_EQ ){
    iReg = sqlite3ExprCodeTarget(pParse, pX.pRight, iTarget);
  }else if( pX.op==TK_ISNULL ){
    iReg = iTarget;
    v.AddOp2(OP_Null, 0, iReg);
  }else{
    int eType;
    int iTab;
    struct InLoop *pIn;

    assert( pX.op==TK_IN );
    iReg = iTarget;
    eType = sqlite3FindInIndex(pParse, pX, 0);
    iTab = pX.iTable;
    v.AddOp2(OP_Rewind, iTab, 0);
    assert( pLevel.plan.wsFlags & WHERE_IN_ABLE );
    if( pLevel.u.in.nIn==0 ){
      pLevel.addrNxt = v.MakeLabel()
    }
    pLevel.u.in.nIn++;
    pLevel.u.in.aInLoop =
       sqlite3DbReallocOrFree(pParse.db, pLevel.u.in.aInLoop,
                              sizeof(pLevel.u.in.aInLoop[0])*pLevel.u.in.nIn);
    pIn = pLevel.u.in.aInLoop;
    if( pIn ){
      pIn += pLevel.u.in.nIn - 1;
      pIn.iCur = iTab;
      if( eType==IN_INDEX_ROWID ){
        pIn.addrInTop = v.AddOp2(OP_Rowid, iTab, iReg);
      }else{
        pIn.addrInTop = v.AddOp3(OP_Column, iTab, 0, iReg);
      }
      v.AddOp1(OP_IsNull, iReg);
    }else{
      pLevel.u.in.nIn = 0;
    }
  }
  disableTerm(pLevel, pTerm);
  return iReg;
}

/*
** Generate code that will evaluate all == and IN constraints for an
** index.
**
** For example, consider table t1(a,b,c,d,e,f) with index i1(a,b,c).
** Suppose the WHERE clause is this:  a==5 AND b IN (1,2,3) AND c>5 AND c<10
** The index has as many as three equality constraints, but in this
** example, the third "c" value is an inequality.  So only two 
** constraints are coded.  This routine will generate code to evaluate
** a==5 and b IN (1,2,3).  The current values for a and b will be stored
** in consecutive registers and the index of the first register is returned.
**
** In the example above nEq==2.  But this subroutine works for any value
** of nEq including 0.  If nEq==0, this routine is nearly a no-op.
** The only thing it does is allocate the pLevel.iMem memory cell and
** compute the affinity string.
**
** This routine always allocates at least one memory cell and returns
** the index of that memory cell. The code that
** calls this routine will use that memory cell to store the termination
** key value of the loop.  If one or more IN operators appear, then
** this routine allocates an additional nEq memory cells for internal
** use.
**
** Before returning, *pzAff is set to point to a buffer containing a
** copy of the column affinity string of the index allocated using
** sqlite3DbMalloc(). Except, entries in the copy of the string associated
** with equality constraints that use NONE affinity are set to
** SQLITE_AFF_NONE. This is to deal with SQL such as the following:
**
**   CREATE TABLE t1(a TEXT PRIMARY KEY, b);
**   SELECT ... FROM t1 AS t2, t1 WHERE t1.a = t2.b;
**
** In the example above, the index on t1(a) has TEXT affinity. But since
** the right hand side of the equality constraint (t2.b) has NONE affinity,
** no conversion should be attempted before using a t2.b value as part of
** a key to search the index. Hence the first byte in the returned affinity
** string in this example would be set to SQLITE_AFF_NONE.
*/
static int codeAllEqualityTerms(
  Parse *pParse,        /* Parsing context */
  WhereLevel *pLevel,   /* Which nested loop of the FROM we are coding */
  WhereClause *pWC,     /* The WHERE clause */
  Bitmask notReady,     /* Which parts of FROM have not yet been coded */
  int nExtraReg,        /* Number of extra registers to allocate */
  char **pzAff          /* OUT: Set to point to affinity string */
){
  int nEq = pLevel.plan.nEq;   /* The number of == or IN constraints to code */
  Vdbe *v = pParse.pVdbe;      /* The vm under construction */
  Index *pIdx;                  /* The index being used for this loop */
  int iCur = pLevel.iTabCur;   /* The cursor of the table */
  WhereTerm *pTerm;             /* A single constraint term */
  int j;                        /* Loop counter */
  int regBase;                  /* Base register */
  int nReg;                     /* Number of registers to allocate */
  char *zAff;                   /* Affinity string to return */

  /* This module is only called on query plans that use an index. */
  assert( pLevel.plan.wsFlags & WHERE_INDEXED );
  pIdx = pLevel.plan.u.pIdx;

  /* Figure out how many memory cells we will need then allocate them.
  */
  regBase = pParse.nMem + 1;
  nReg = pLevel.plan.nEq + nExtraReg;
  pParse.nMem += nReg;

  zAff = sqlite3DbStrDup(pParse.db, v.IndexAffinityStr(pIdx))
  if( !zAff ){
    pParse.db.mallocFailed = true
  }

  /* Evaluate the equality constraints
  */
  assert( len(pIdx.Columns) >= nEq )
  for(j=0; j<nEq; j++){
    int r1;
    int k = pIdx.Columns[j];
    pTerm = findTerm(pWC, iCur, k, notReady, pLevel.plan.wsFlags, pIdx);
    if( pTerm==0 ) break;
    /* The following true for indices with redundant columns. 
    ** Ex: CREATE INDEX i1 ON t1(a,b,a); SELECT * FROM t1 WHERE a=0 AND b=0; */
    r1 = codeEqualityTerm(pParse, pTerm, pLevel, regBase+j);
    if( r1!=regBase+j ){
      if( nReg==1 ){
        pParse.ReleaseTempReg(regBase)
        regBase = r1;
      }else{
        v.AddOp2(OP_SCopy, r1, regBase+j);
      }
    }
    if( (pTerm.eOperator & (WO_ISNULL|WO_IN))==0 ){
      Expr *pRight = pTerm.Expr.pRight;
      sqlite3ExprCodeIsNullJump(v, pRight, regBase+j, pLevel.addrBrk);
      if( zAff ){
        if( sqlite3CompareAffinity(pRight, zAff[j])==SQLITE_AFF_NONE ){
          zAff[j] = SQLITE_AFF_NONE;
        }
        if( sqlite3ExprNeedsNoAffinityChange(pRight, zAff[j]) ){
          zAff[j] = SQLITE_AFF_NONE;
        }
      }
    }
  }
  *pzAff = zAff;
  return regBase;
}

#ifndef SQLITE_OMIT_EXPLAIN
//	This routine is a helper for explainIndexRange() below
//	pStr holds the text of an expression that we are building up one term at a time. This routine adds a new term to the end of the expression. Terms are separated by AND so add the "AND" text for second and subsequent terms only.
func explainAppendTerm(text string, iTerm int, zColumn, zOp string) string {
	s := text
	if iTerm != 0 {
		s = append(s, " AND ")
	}
	return append(s, zColumn, zOp, "?")
}

/*
** Argument pLevel describes a strategy for scanning table pTab. This 
** function returns a pointer to a string buffer containing a description
** of the subset of table rows scanned by the strategy in the form of an
** SQL expression. Or, if all rows are scanned, NULL is returned.
**
** For example, if the query:
**
**   SELECT * FROM t1 WHERE a=1 AND b>2;
**
** is run and there is an index on (a, b), then this function returns a
** string similar to:
**
**   "a=? AND b>?"
**
** The returned pointer points to memory obtained from sqlite3DbMalloc().
** It is the responsibility of the caller to free the buffer when it is
** no longer required.
*/
static char *explainIndexRange(sqlite3 *db, WhereLevel *pLevel, Table *pTab){
  WherePlan *pPlan = &pLevel.plan;
  Index *pIndex = pPlan.u.pIdx;
  int nEq = pPlan.nEq;
  int i, j;
  Column *aCol = pTab.Columns;
  Columns := pIndex.Columns

  if( nEq==0 && (pPlan.wsFlags & (WHERE_BTM_LIMIT|WHERE_TOP_LIMIT))==0 ){
    return 0;
  }
  txt := " ("
  for(i=0; i<nEq; i++){
    txt = explainAppendTerm(txt, i, aCol[Columns[i]].Name, "=");
  }

  j = i;
  if( pPlan.wsFlags&WHERE_BTM_LIMIT ){
    char *z = (j == len(pIndex.Columns) ) ? "rowid" : aCol[Columns[j]].Name
    txt = explainAppendTerm(txt, i++, z, ">")
  }
  if( pPlan.wsFlags&WHERE_TOP_LIMIT ){
    char *z = (j == len(pIndex.Columns) ) ? "rowid" : aCol[Columns[j]].Name
    txt = explainAppendTerm(txt, i, z, "<")
  }
  txt = append(txt, ")")
  return txt
}

/*
** This function is a no-op unless currently processing an EXPLAIN QUERY PLAN
** command. If the query being compiled is an EXPLAIN QUERY PLAN, a single
** record is added to the output to describe the table scan strategy in 
** pLevel.
*/
static void explainOneScan(
  Parse *pParse,                  /* Parse context */
  SrcList *pTabList,              /* Table list this loop refers to */
  WhereLevel *pLevel,             /* Scan to write OP_Explain opcode for */
  int iLevel,                     /* Value for "level" column of output */
  int iFrom,                      /* Value for "from" column of output */
  uint16 Flags                  /* Flags passed to WhereBegin() */
){
  if( pParse.explain==2 ){
    uint32 flags = pLevel.plan.wsFlags;
    struct SrcList_item *pItem = &pTabList.a[pLevel.iFrom];
    Vdbe *v = pParse.pVdbe;      /* VM being constructed */
    sqlite3 *db = pParse.db;     /* Database handle */
    char *zMsg;                   /* Text to add to EQP output */
    int64 nRow;           /* Expected number of rows visited by scan */
    int iId = pParse.iSelectId;  /* Select id (left-most output column) */
    int isSearch;                 /* True for a SEARCH. False for SCAN. */

    if( (flags&WHERE_MULTI_OR) || (Flags&WHERE_ONETABLE_ONLY) ) return;

    isSearch = (pLevel.plan.nEq>0)
             || (flags&(WHERE_BTM_LIMIT|WHERE_TOP_LIMIT))!=0
             || (Flags&(WHERE_ORDERBY_MIN|WHERE_ORDERBY_MAX));

	if isSearch {
		zMsg = "SEARCH"
	} else {
		zMsg = "SCAN"
	}
    if( pItem.Select ){
      zMsg = fmt.Sprintf("%v SUBQUERY %v", zMsg, pItem.iSelectId);
    }else{
      zMsg = fmt.Sprintf("%v TABLE %v", zMsg, pItem.Name);
    }

    if( pItem.zAlias ){
      zMsg = fmt.Sprintf("%v AS %v", zMsg, pItem.zAlias);
    }
    if( (flags & WHERE_INDEXED)!=0 ){
      char *zWhere = explainIndexRange(db, pLevel, pItem.pTab);
      zMsg = fmt.Sprintf("%v USING %v%vINDEX%v%v%v", zMsg, 
          ((flags & WHERE_TEMP_INDEX)?"AUTOMATIC ":""),
          ((flags & WHERE_IDX_ONLY)?"COVERING ":""),
          ((flags & WHERE_TEMP_INDEX)?"":" "),
          ((flags & WHERE_TEMP_INDEX)?"": pLevel.plan.u.pIdx.Name),
          zWhere
      );
      zWhere = nil
    }else if( flags & (WHERE_ROWID_EQ|WHERE_ROWID_RANGE) ){
      zMsg = fmt.Sprintf("%v USING INTEGER PRIMARY KEY", zMsg);

      if( flags&WHERE_ROWID_EQ ){
        zMsg = fmt.Sprintf("%v (rowid=?)", zMsg);
      }else if( (flags&WHERE_BOTH_LIMIT)==WHERE_BOTH_LIMIT ){
        zMsg = fmt.Sprintf("%v (rowid>? AND rowid<?)", zMsg);
      }else if( flags&WHERE_BTM_LIMIT ){
        zMsg = fmt.Sprintf("%v (rowid>?)", zMsg);
      }else if( flags&WHERE_TOP_LIMIT ){
        zMsg = fmt.Sprintf("%v (rowid<?)", zMsg);
      }
    }
    else if( (flags & WHERE_VIRTUALTABLE)!=0 ){
      sqlite3_index_info *pVtabIdx = pLevel.plan.u.pVtabIdx;
      zMsg = fmt.Sprintf("%v VIRTUAL TABLE INDEX %v:%v", zMsg, pVtabIdx.idxNum, pVtabIdx.idxStr);
    }
    if( Flags&(WHERE_ORDERBY_MIN|WHERE_ORDERBY_MAX) ){
      nRow = 1;
    }else{
      nRow = (int64)pLevel.plan.nRow;
    }
    zMsg = fmt.Sprintf("%v (~%v rows)", zMsg, nRow);
    sqlite3VdbeAddOp4(v, OP_Explain, iId, iLevel, iFrom, zMsg, P4_DYNAMIC);
  }
}
#else
# define explainOneScan(u,v,w,x,y,z)
#endif /* SQLITE_OMIT_EXPLAIN */


/*
** Generate code for the start of the iLevel-th loop in the WHERE clause
** implementation described by pWInfo.
*/
static Bitmask codeOneLoopStart(
  WhereInfo *pWInfo,   /* Complete information about the WHERE clause */
  int iLevel,          /* Which level of pWInfo.a[] should be coded */
  uint16 Flags,      /* One of the WHERE_* flags defined in sqliteInt.h */
  Bitmask notReady     /* Which tables are currently available */
){
  int j, k;            /* Loop counters */
  int iCur;            /* The VDBE cursor for the table */
  int addrNxt;         /* Where to jump to continue with the next IN case */
  int omitTable;       /* True if we use the index only */
  int bRev;            /* True if we need to scan in reverse order */
  WhereLevel *pLevel;  /* The where level to be coded */
  WhereClause *pWC;    /* Decomposition of the entire WHERE clause */
  WhereTerm *pTerm;               /* A WHERE clause term */
  Parse *pParse;                  /* Parsing context */
  Vdbe *v;                        /* The prepared stmt under constructions */
  struct SrcList_item *pTabItem;  /* FROM clause term being coded */
  int addrBrk;                    /* Jump here to break out of the loop */
  int addrCont;                   /* Jump here to continue with next cycle */
  int iRowidReg = 0;        /* Rowid is stored in this register, if not zero */
  int iReleaseReg = 0;      /* Temp register to free before returning */

  pParse = pWInfo.pParse;
  v = pParse.pVdbe;
  pWC = pWInfo.pWC;
  pLevel = &pWInfo.a[iLevel];
  pTabItem = &pWInfo.pTabList.a[pLevel.iFrom];
  iCur = pTabItem.iCursor;
  bRev = (pLevel.plan.wsFlags & WHERE_REVERSE)!=0;
  omitTable = (pLevel.plan.wsFlags & WHERE_IDX_ONLY)!=0 
           && (Flags & WHERE_FORCE_TABLE)==0;

  /* Create labels for the "break" and "continue" instructions
  ** for the current loop.  Jump to addrBrk to break out of a loop.
  ** Jump to cont to go immediately to the next iteration of the
  ** loop.
  **
  ** When there is an IN operator, we also have a "addrNxt" label that
  ** means to continue with the next IN value combination.  When
  ** there are no IN operators in the constraints, the "addrNxt" label
  ** is the same as "addrBrk".
  */
  addrBrk = pLevel.addrBrk = pLevel.addrNxt = v.MakeLabel()
  addrCont = pLevel.addrCont = v.MakeLabel()

  /* If this is the right table of a LEFT OUTER JOIN, allocate and
  ** initialize a memory cell that records if this table matches any
  ** row of the left table of the join.
  */
  if( pLevel.iFrom>0 && (pTabItem[0].jointype & JT_LEFT)!=0 ){
    pLevel.iLeftJoin = ++pParse.nMem;
    v.AddOp2(OP_Integer, 0, pLevel.iLeftJoin);
    v.Comment("init LEFT JOIN no-match flag")
  }

  if(  (pLevel.plan.wsFlags & WHERE_VIRTUALTABLE)!=0 ){
    /* Case 0:  The table is a virtual-table.  Use the VFilter and VNext
    **          to access the data.
    */
    int iReg;   /* P3 Value for OP_VFilter */
	pVtabIdx := pLevel.plan.u.pVtabIdx
	nConstraint := pVtabIdx.nConstraint
	aUsage := pVtabIdx.aConstraintUsage
	aConstraint := pVtabIdx.aConstraint

	pParse.ColumnCacheContext(func() {
		iReg = pParse.GetTempRange(nConstraint + 2)
		for j = 1; j <= nConstraint; j++ {
			for k = 0; k < nConstraint; k++ {
				if aUsage[k].argvIndex == j {
        			iTerm := aConstraint[k].iTermOffset
        			sqlite3ExprCode(pParse, pWC.Terms[iTerm].Expr.pRight, iReg + j + 1)
					break
				}
			}
			if k == nConstraint {
				break
			}
    	}
    	v.AddOp2(OP_Integer, pVtabIdx.idxNum, iReg)
		v.AddOp2(OP_Integer, j - 1, iReg + 1)
    	sqlite3VdbeAddOp4(v, OP_VFilter, iCur, addrBrk, iReg, pVtabIdx.idxStr, pVtabIdx.needToFreeIdxStr ? P4_MPRINTF : P4_STATIC)
		pVtabIdx.needToFreeIdxStr = 0
		for(j=0; j<nConstraint; j++){
    		if aUsage[j].omit {
        		int iTerm = aConstraint[j].iTermOffset
        		disableTerm(pLevel, &pWC.Terms[iTerm])
			}
		}
		pLevel.op = OP_VNext
    	pLevel.p1 = iCur
    	pLevel.p2 = v.CurrentAddr()
    	pParse.ReleaseTempRange(iReg, nConstraint + 2)
    })
  } else if( pLevel.plan.wsFlags & WHERE_ROWID_EQ ){
    /* Case 1:  We can directly reference a single row using an
    **          equality comparison against the ROWID field.  Or
    **          we reference multiple rows using a "rowid IN (...)"
    **          construct.
    */
    iReleaseReg = pParse.GetTempReg()
    pTerm = findTerm(pWC, iCur, -1, notReady, WO_EQ|WO_IN, 0);
    assert( pTerm!=0 );
    assert( pTerm.Expr!=0 );
    assert( pTerm.leftCursor==iCur );
    assert( omitTable==0 );
    iRowidReg = codeEqualityTerm(pParse, pTerm, pLevel, iReleaseReg);
    addrNxt = pLevel.addrNxt;
    v.AddOp2(OP_MustBeInt, iRowidReg, addrNxt);
    v.AddOp3(OP_NotExists, iCur, addrNxt, iRowidReg);
    sqlite3ExprCacheStore(pParse, iCur, -1, iRowidReg);
    v.Comment("pk")
    pLevel.op = OP_Noop;
  }else if( pLevel.plan.wsFlags & WHERE_ROWID_RANGE ){
    /* Case 2:  We have an inequality comparison against the ROWID field.
    */
    int testOp = OP_Noop;
    int start;
    int memEndValue = 0;
    WhereTerm *pStart, *pEnd;

    assert( omitTable==0 );
    pStart = findTerm(pWC, iCur, -1, notReady, WO_GT|WO_GE, 0);
    pEnd = findTerm(pWC, iCur, -1, notReady, WO_LT|WO_LE, 0);
    if( bRev ){
      pTerm = pStart;
      pStart = pEnd;
      pEnd = pTerm;
    }
    if( pStart ){
      Expr *pX;             /* The expression that defines the start bound */
      int r1, rTemp;        /* Registers for holding the start boundary */

      /* The following constant maps TK_xx codes into corresponding 
      ** seek opcodes.  It depends on a particular ordering of TK_xx
      */
      const byte aMoveOp[] = {
           /* TK_GT */  OP_SeekGt,
           /* TK_LE */  OP_SeekLe,
           /* TK_LT */  OP_SeekLt,
           /* TK_GE */  OP_SeekGe
      };
      assert( TK_LE==TK_GT+1 );      /* Make sure the ordering.. */
      assert( TK_LT==TK_GT+2 );      /*  ... of the TK_xx values... */
      assert( TK_GE==TK_GT+3 );      /*  ... is correcct. */

      pX = pStart.Expr;
      assert( pX!=0 );
      assert( pStart.leftCursor==iCur );
      r1 = sqlite3ExprCodeTemp(pParse, pX.pRight, &rTemp);
      v.AddOp3(aMoveOp[pX.op-TK_GT], iCur, addrBrk, r1);
      v.Comment("pk")
      sqlite3ExprCacheAffinityChange(pParse, r1, 1);
      pParse.ReleaseTempReg(rTemp)
      disableTerm(pLevel, pStart);
    }else{
      v.AddOp2(bRev ? OP_Last : OP_Rewind, iCur, addrBrk);
    }
    if( pEnd ){
      Expr *pX;
      pX = pEnd.Expr;
      assert( pX!=0 );
      assert( pEnd.leftCursor==iCur );
      memEndValue = ++pParse.nMem;
      sqlite3ExprCode(pParse, pX.pRight, memEndValue);
      if( pX.op==TK_LT || pX.op==TK_GT ){
        testOp = bRev ? OP_Le : OP_Ge;
      }else{
        testOp = bRev ? OP_Lt : OP_Gt;
      }
      disableTerm(pLevel, pEnd);
    }
    start = v.CurrentAddr()
    pLevel.op = bRev ? OP_Prev : OP_Next;
    pLevel.p1 = iCur;
    pLevel.p2 = start;
    if( pStart==0 && pEnd==0 ){
      pLevel.p5 = SQLITE_STMTSTATUS_FULLSCAN_STEP;
    }else{
      assert( pLevel.p5==0 );
    }
    if( testOp!=OP_Noop ){
      iRowidReg = iReleaseReg = pParse.GetTempReg()
      v.AddOp2(OP_Rowid, iCur, iRowidReg);
      sqlite3ExprCacheStore(pParse, iCur, -1, iRowidReg);
      v.AddOp3(testOp, memEndValue, addrBrk, iRowidReg)
      v.ChangeP5(SQLITE_AFF_NUMERIC | SQLITE_JUMPIFNULL)
    }
  }else if( pLevel.plan.wsFlags & (WHERE_COLUMN_RANGE|WHERE_COLUMN_EQ) ){
    /* Case 3: A scan using an index.
    **
    **         The WHERE clause may contain zero or more equality 
    **         terms ("==" or "IN" operators) that refer to the N
    **         left-most columns of the index. It may also contain
    **         inequality constraints (>, <, >= or <=) on the indexed
    **         column that immediately follows the N equalities. Only 
    **         the right-most column can be an inequality - the rest must
    **         use the "==" and "IN" operators. For example, if the 
    **         index is on (x,y,z), then the following clauses are all 
    **         optimized:
    **
    **            x=5
    **            x=5 AND y=10
    **            x=5 AND y<10
    **            x=5 AND y>5 AND y<10
    **            x=5 AND y=5 AND z<=10
    **
    **         The z<10 term of the following cannot be used, only
    **         the x=5 term:
    **
    **            x=5 AND z<10
    **
    **         N may be zero if there are inequality constraints.
    **         If there are no inequality constraints, then N is at
    **         least one.
    **
    **         This case is also used when there are no WHERE clause
    **         constraints but an index is selected anyway, in order
    **         to force the output order to conform to an ORDER BY.
    */  
    static const byte aStartOp[] = {
      0,
      0,
      OP_Rewind,           /* 2: (!start_constraints && startEq &&  !bRev) */
      OP_Last,             /* 3: (!start_constraints && startEq &&   bRev) */
      OP_SeekGt,           /* 4: (start_constraints  && !startEq && !bRev) */
      OP_SeekLt,           /* 5: (start_constraints  && !startEq &&  bRev) */
      OP_SeekGe,           /* 6: (start_constraints  &&  startEq && !bRev) */
      OP_SeekLe            /* 7: (start_constraints  &&  startEq &&  bRev) */
    };
    static const byte aEndOp[] = {
      OP_Noop,             /* 0: (!end_constraints) */
      OP_IdxGE,            /* 1: (end_constraints && !bRev) */
      OP_IdxLT             /* 2: (end_constraints && bRev) */
    };
    int nEq = pLevel.plan.nEq;  /* Number of == or IN terms */
    int isMinQuery = 0;          /* If this is an optimized SELECT min(x).. */
    int regBase;                 /* Base register holding constraint values */
    int r1;                      /* Temp register */
    WhereTerm *pRangeStart = 0;  /* Inequality constraint at range start */
    WhereTerm *pRangeEnd = 0;    /* Inequality constraint at range end */
    int startEq;                 /* True if range start uses ==, >= or <= */
    int endEq;                   /* True if range end uses ==, >= or <= */
    int start_constraints;       /* Start of range is constrained */
    int nConstraint;             /* Number of constraint terms */
    Index *pIdx;                 /* The index we will be using */
    int iIdxCur;                 /* The VDBE cursor for the index */
    int nExtraReg = 0;           /* Number of extra registers needed */
    int op;                      /* Instruction opcode */
    char *zStartAff;             /* Affinity for start of range constraint */
    char *zEndAff;               /* Affinity for end of range constraint */

    pIdx = pLevel.plan.u.pIdx;
    iIdxCur = pLevel.iIdxCur;
    k = (nEq == len(pIdx.Columns) ? -1 : pIdx.Columns[nEq])

    /* If this loop satisfies a sort order (pOrderBy) request that 
    ** was passed to this function to implement a "SELECT min(x) ..." 
    ** query, then the caller will only allow the loop to run for
    ** a single iteration. This means that the first row returned
    ** should not have a NULL value stored in 'x'. If column 'x' is
    ** the first one after the nEq equality constraints in the index,
    ** this requires some special handling.
    */
    if (Flags & WHERE_ORDERBY_MIN) != 0 && (pLevel.plan.wsFlags & WHERE_ORDERBY) && (len(pIdx.Columns) > nEq) {
      /* assert( pOrderBy.Len() == 1 ); */
      /* assert( pOrderBy.Items[0].Expr.iColumn == pIdx.Columns[nEq] ); */
      isMinQuery = 1;
      nExtraReg = 1;
    }

    /* Find any inequality constraint terms for the start and end 
    ** of the range. 
    */
    if( pLevel.plan.wsFlags & WHERE_TOP_LIMIT ){
      pRangeEnd = findTerm(pWC, iCur, k, notReady, (WO_LT|WO_LE), pIdx);
      nExtraReg = 1;
    }
    if( pLevel.plan.wsFlags & WHERE_BTM_LIMIT ){
      pRangeStart = findTerm(pWC, iCur, k, notReady, (WO_GT|WO_GE), pIdx);
      nExtraReg = 1;
    }

    /* Generate code to evaluate all constraint terms using == or IN
    ** and store the values of those terms in an array of registers
    ** starting at regBase.
    */
    regBase = codeAllEqualityTerms(
        pParse, pLevel, pWC, notReady, nExtraReg, &zStartAff
    );
    zEndAff = sqlite3DbStrDup(pParse.db, zStartAff);
    addrNxt = pLevel.addrNxt;

    //	If we are doing a reverse order scan on an ascending index, or a forward order scan on a descending index, interchange the start and end terms (pRangeStart and pRangeEnd).
    if (nEq < len(pIdx.Columns) && bRev == (pIdx.aSortOrder[nEq] == SQLITE_SO_ASC)) || (bRev && len(pIdx.Columns) == nEq) {
      pRangeEnd, pRangeStart = pRangeStart, pRangeEnd
    }

    startEq = !pRangeStart || pRangeStart.eOperator & (WO_LE|WO_GE);
    endEq =   !pRangeEnd || pRangeEnd.eOperator & (WO_LE|WO_GE);
    start_constraints = pRangeStart || nEq>0;

    /* Seek the index cursor to the start of the range. */
    nConstraint = nEq;
    if( pRangeStart ){
      Expr *pRight = pRangeStart.Expr.pRight;
      sqlite3ExprCode(pParse, pRight, regBase+nEq);
      if( (pRangeStart.wtFlags & TERM_VNULL)==0 ){
        sqlite3ExprCodeIsNullJump(v, pRight, regBase+nEq, addrNxt);
      }
      if( zStartAff ){
        if( sqlite3CompareAffinity(pRight, zStartAff[nEq])==SQLITE_AFF_NONE){
          /* Since the comparison is to be performed with no conversions
          ** applied to the operands, set the affinity to apply to pRight to 
          ** SQLITE_AFF_NONE.  */
          zStartAff[nEq] = SQLITE_AFF_NONE;
        }
        if( sqlite3ExprNeedsNoAffinityChange(pRight, zStartAff[nEq]) ){
          zStartAff[nEq] = SQLITE_AFF_NONE;
        }
      }  
      nConstraint++;
    }else if( isMinQuery ){
      v.AddOp2(OP_Null, 0, regBase+nEq);
      nConstraint++;
      startEq = 0;
      start_constraints = 1;
    }
    codeApplyAffinity(pParse, regBase, nConstraint, zStartAff);
    op = aStartOp[(start_constraints<<2) + (startEq<<1) + bRev];
    assert( op!=0 );
    sqlite3VdbeAddOp4Int(v, op, iIdxCur, addrNxt, regBase, nConstraint);

    /* Load the value for the inequality constraint at the end of the
    ** range (if any).
    */
    nConstraint = nEq;
    if( pRangeEnd ){
      Expr *pRight = pRangeEnd.Expr.pRight;
      pParse.ExprCacheRemove(regBase + nEq, 1)
      sqlite3ExprCode(pParse, pRight, regBase+nEq);
      if( (pRangeEnd.wtFlags & TERM_VNULL)==0 ){
        sqlite3ExprCodeIsNullJump(v, pRight, regBase+nEq, addrNxt);
      }
      if( zEndAff ){
        if( sqlite3CompareAffinity(pRight, zEndAff[nEq])==SQLITE_AFF_NONE){
          /* Since the comparison is to be performed with no conversions
          ** applied to the operands, set the affinity to apply to pRight to 
          ** SQLITE_AFF_NONE.  */
          zEndAff[nEq] = SQLITE_AFF_NONE;
        }
        if( sqlite3ExprNeedsNoAffinityChange(pRight, zEndAff[nEq]) ){
          zEndAff[nEq] = SQLITE_AFF_NONE;
        }
      }  
      codeApplyAffinity(pParse, regBase, nEq+1, zEndAff);
      nConstraint++;
    }
    zStartAff = nil
    zEndAff = nil

    /* Top of the loop body */
    pLevel.p2 = v.CurrentAddr()

    /* Check if the index cursor is past the end of the range. */
    op = aEndOp[(pRangeEnd || nEq) * (1 + bRev)];
    if( op!=OP_Noop ){
      sqlite3VdbeAddOp4Int(v, op, iIdxCur, addrNxt, regBase, nConstraint);
      v.ChangeP5(endEq != bRev ? 1 : 0)
    }

    /* If there are inequality constraints, check that the value
    ** of the table column that the inequality contrains is not NULL.
    ** If it is, jump to the next iteration of the loop.
    */
    r1 = pParse.GetTempReg()
    if( (pLevel.plan.wsFlags & (WHERE_BTM_LIMIT|WHERE_TOP_LIMIT))!=0 ){
      v.AddOp3(OP_Column, iIdxCur, nEq, r1)
      v.AddOp2(OP_IsNull, r1, addrCont);
    }
    pParse.ReleaseTempReg(r1)

    /* Seek the table cursor, if required */
    disableTerm(pLevel, pRangeStart);
    disableTerm(pLevel, pRangeEnd);
    if( !omitTable ){
      iRowidReg = iReleaseReg = pParse.GetTempReg()
      v.AddOp2(OP_IdxRowid, iIdxCur, iRowidReg);
      sqlite3ExprCacheStore(pParse, iCur, -1, iRowidReg);
      v.AddOp2(OP_Seek, iCur, iRowidReg);  /* Deferred seek */
    }

    /* Record the instruction used to terminate the loop. Disable 
    ** WHERE clause terms made redundant by the index range scan.
    */
    if( pLevel.plan.wsFlags & WHERE_UNIQUE ){
      pLevel.op = OP_Noop;
    }else if( bRev ){
      pLevel.op = OP_Prev;
    }else{
      pLevel.op = OP_Next;
    }
    pLevel.p1 = iIdxCur;
  }else

#ifndef SQLITE_OMIT_OR_OPTIMIZATION
  if( pLevel.plan.wsFlags & WHERE_MULTI_OR ){
    /* Case 4:  Two or more separately indexed terms connected by OR
    **
    ** Example:
    **
    **   CREATE TABLE t1(a,b,c,d);
    **   CREATE INDEX i1 ON t1(a);
    **   CREATE INDEX i2 ON t1(b);
    **   CREATE INDEX i3 ON t1(c);
    **
    **   SELECT * FROM t1 WHERE a=5 OR b=7 OR (c=11 AND d=13)
    **
    ** In the example, there are three indexed terms connected by OR.
    ** The top of the loop looks like this:
    **
    **          Null       1                # Zero the rowset in reg 1
    **
    ** Then, for each indexed term, the following. The arguments to
    ** RowSetTest are such that the rowid of the current row is inserted
    ** into the RowSet. If it is already present, control skips the
    ** Gosub opcode and jumps straight to the code generated by WhereEnd().
    **
    **        WhereBegin(<term>)
    **          RowSetTest                  # Insert rowid into rowset
    **          Gosub      2 A
    **        sqlite3WhereEnd()
    **
    ** Following the above, code to terminate the loop. Label A, the target
    ** of the Gosub above, jumps to the instruction right after the Goto.
    **
    **          Null       1                # Zero the rowset in reg 1
    **          Goto       B                # The loop is finished.
    **
    **       A: <loop body>                 # Return data, whatever.
    **
    **          Return     2                # Jump back to the Gosub
    **
    **       B: <after the loop>
    **
    */
    WhereClause *pOrWc;    /* The OR-clause broken out into subterms */

    int regReturn = ++pParse.nMem;           /* Register used with OP_Gosub */
    int regRowset = 0;                        /* Register for RowSet object */
    int regRowid = 0;                         /* Register holding rowid */
	iLoopBody := v.MakeLabel()				//	Start of loop body
    int iRetInit;                             /* Address of regReturn init */
    int untestedTerms = 0;             /* Some terms not completely tested */
    int ii;                            /* Loop counter */
    Expr *pAndExpr = 0;                /* An ".. AND (...)" expression */
   
    pTerm = pLevel.plan.u.pTerm;
    assert( pTerm!=0 );
    assert( pTerm.eOperator==WO_OR );
    assert( (pTerm.wtFlags & TERM_ORINFO)!=0 );
    pOrWc = &pTerm.u.pOrInfo.wc;
    pLevel.op = OP_Return;
    pLevel.p1 = regReturn;

	//	Set up a new SrcList ni pOrTab containing the table being scanned by this loop in the a[0] slot and all notReady tables in a[1..] slots. This becomes the SrcList in the recursive call to WhereBegin().
    pOrTab			SrcList					//	Shortened table list or OR-clause generation
    if pWInfo.nLevel > 1 {
		nNotReady := pWInfo.nLevel - iLevel - 1
		pOrTab = make(SrcList, 1, nNotReady)
		if pOrTab == nil {
			return notReady
		}
		pOrTab[0] = pTabItem
		origSrc := pWInfo.pTabList
		for k := 1; k <= nNotReady; k++ {
			pOrTab = append(pOrTab, origSrc[pLevel[k].iFrom])
		}
	} else {
		pOrTab = pWInfo.pTabList
	}

	//	Initialize the rowset register to contain NULL. An SQL NULL is equivalent to an empty rowset.
	//	Also initialize regReturn to contain the address of the instruction immediately following the OP_Return at the bottom of the loop. This is required in a few obscure LEFT JOIN cases where control jumps over the top of the loop into the body of it. In this case the correct response for the end-of-loop code (the OP_Return) is to fall through to the next instruction, just as an OP_Next does if called on an uninitialized cursor.
	if (Flags & WHERE_DUPLICATES_OK) == 0 {
		regRowset = ++pParse.nMem
		regRowid = ++pParse.nMem
		v.AddOp2(OP_Null, 0, regRowset)
	}
    iRetInit = v.AddOp2(OP_Integer, 0, regReturn)

	//	If the original WHERE clause is z of the form:  (x1 OR x2 OR ...) AND y
	//	Then for every term xN, evaluate as the subexpression: xN AND z
	//	That way, terms in y that are factored into the disjunction will be picked up by the recursive calls to WhereBegin() below.
	//	Actually, each subexpression is converted to "xN AND w" where w is the "interesting" terms of z - terms that did not originate in the ON or USING clause of a LEFT JOIN, and terms that are usable as indices.
	if len(pWC.Terms) > 1 {
		for i, term := range pWC.Terms {
			pExpr := term.Expr
			if pExpr.HasProperty(EP_FromJoin) {
				continue
			}
			if term.wtFlags & (TERM_VIRTUAL | TERM_ORINFO) {
				continue
			}
			if (term.eOperator & WO_ALL) == 0 {
				continue
			}
			pAndExpr = pParse.db.ExprAnd(pAndExpr, pExpr.Dup())
		}
		if pAndExpr {
			pAndExpr = pParse.Expr(TK_AND, nil, pAndExpr, "")
		}
	}

	for ii := 0; ii < pOrWc.nTerm; ii++ {
		WhereTerm *pOrTerm = &pOrWc.a[ii];
		if pOrTerm.leftCursor == iCur || pOrTerm.eOperator == WO_AND {
			pOrExpr := pOrTerm.Expr
			if pAndExpr {
				pAndExpr.pLeft = pOrExpr
				pOrExpr = pAndExpr
			}
			//	Loop through table entries that match term pOrTerm.
			if pSubWInfo := pParse.WhereBegin(pOrTab, pOrExpr, nil, nil, WHERE_OMIT_OPEN_CLOSE | WHERE_AND_ONLY | WHERE_FORCE_TABLE | WHERE_ONETABLE_ONLY); pSubWInfo != nil {
				explainOneScan(pParse, pOrTab, &pSubWInfo.a[0], iLevel, pLevel.iFrom, 0)
				if (Flags & WHERE_DUPLICATES_OK) == 0 {
					r := pParse.ExprCodeGetColumn(pTabItem.pTab, -1, iCur, regRowid, 0)
					if ii == pOrWc.nTerm - 1 {
						sqlite3VdbeAddOp4Int(v, OP_RowSetTest, regRowset, v.CurrentAddr() + 2, r, -1)
					} else {
						sqlite3VdbeAddOp4Int(v, OP_RowSetTest, regRowset, v.CurrentAddr() + 2, r, ii)
					}
				}
				v.AddOp2(OP_Gosub, regReturn, iLoopBody)

				//	The pSubWInfo.untestedTerms flag means that this OR term contained one or more AND term from a notReady table. The terms from the notReady table could not be tested and will need to be tested later.
				if pSubWInfo.untestedTerms {
					untestedTerms = 1
				}
				//	Finish the loop through table entries that match term pOrTerm.
				sqlite3WhereEnd(pSubWInfo)
				}
			}
		}
		if pAndExpr != nil {
			pAndExpr.pLeft = nil
			pParse.db.ExprDelete(pAndExpr)
		}
		v.ChangeP1(iRetInit, v.CurrentAddr())
		v.AddOp2(OP_Goto, 0, pLevel.addrBrk)
		v.ResolveLabel(iLoopBody)

		if pWInfo.nLevel > 1 {
			sqlite3StackFree(pParse.db, pOrTab)
		}
		if !untestedTerms {
			disableTerm(pLevel, pTerm)
		}
	} else
#endif /* SQLITE_OMIT_OR_OPTIMIZATION */

  {
    /* Case 5:  There is no usable index.  We must do a complete
    **          scan of the entire table.
    */
    static const byte aStep[] = { OP_Next, OP_Prev };
    static const byte aStart[] = { OP_Rewind, OP_Last };
    assert( bRev==0 || bRev==1 );
    assert( omitTable==0 );
    pLevel.op = aStep[bRev];
    pLevel.p1 = iCur;
    pLevel.p2 = 1 + v.AddOp2(aStart[bRev], iCur, addrBrk);
    pLevel.p5 = SQLITE_STMTSTATUS_FULLSCAN_STEP;
  }
  notReady &= ~getMask(pWC.WhereMaskSet, iCur);

  /* Insert code to test every subexpression that can be completely
  ** computed using the current set of tables.
  **
  ** IMPLEMENTATION-OF: R-49525-50935 Terms that cannot be satisfied through
  ** the use of indices become tests that are evaluated against each row of
  ** the relevant input tables.
  */
  for pTerm=pWC.Terms[0], j = len(pWC.Terms); j>0; j--, pTerm++ {
    Expr *pE;
    if( pTerm.wtFlags & (TERM_VIRTUAL|TERM_CODED) ) continue;
    if( (pTerm.prereqAll & notReady)!=0 ){
      pWInfo.untestedTerms = 1;
      continue;
    }
    pE = pTerm.Expr;
    assert( pE!=0 );
    if( pLevel.iLeftJoin && !pE.HasProperty(EP_FromJoin) ){
      continue;
    }
    sqlite3ExprIfFalse(pParse, pE, addrCont, SQLITE_JUMPIFNULL);
    pTerm.wtFlags |= TERM_CODED;
  }

  /* For a LEFT OUTER JOIN, generate code that will record the fact that
  ** at least one row of the right table has matched the left table.  
  */
  if( pLevel.iLeftJoin ){
    pLevel.addrFirst = v.CurrentAddr()
    v.AddOp2(OP_Integer, 1, pLevel.iLeftJoin);
    v.Comment("record LEFT JOIN hit")
    pParse.ExprCacheClear()
    for pTerm = pWC.Terms[0], j=0; j < len(pWC.Terms); j++, pTerm++ {
      if( pTerm.wtFlags & (TERM_VIRTUAL|TERM_CODED) ) continue;
      if( (pTerm.prereqAll & notReady)!=0 ){
        assert( pWInfo.untestedTerms );
        continue;
      }
      assert( pTerm.Expr );
      sqlite3ExprIfFalse(pParse, pTerm.Expr, addrCont, SQLITE_JUMPIFNULL);
      pTerm.wtFlags |= TERM_CODED;
    }
  }
  pParse.ReleaseTempReg(iReleaseReg)

  return notReady;
}

/*
** Free a WhereInfo structure
*/
static void whereInfoFree(sqlite3 *db, WhereInfo *pWInfo) {
  if pWInfo {
    int i;
    for(i=0; i<pWInfo.nLevel; i++){
      sqlite3_index_info *pInfo = pWInfo.a[i].pIdxInfo;
      if( pInfo ){
        /* assert( pInfo.needToFreeIdxStr==0 || db.mallocFailed ); */
        if( pInfo.needToFreeIdxStr ){
          pInfo.idxStr = nil
        }
        pInfo = nil
      }
      if( pWInfo.a[i].plan.wsFlags & WHERE_TEMP_INDEX ){
        Index *pIdx = pWInfo.a[i].plan.u.pIdx;
        if( pIdx ){
          pIdx.zColAff = nil
          pIdx = nil
        }
      }
    }
    pWInfo.pWC.Clear()
    pWInfo = nil
  }
}


//	Generate the beginning of the loop used for WHERE clause processing. The return value is a pointer to an opaque structure that contains information needed to terminate the loop. Later, the calling routine should invoke sqlite3WhereEnd() with the return value of this function in order to complete the WHERE clause processing.
//	If an error occurs, this routine returns nil.
//	The basic idea is to do a nested loop, one loop for each table in the FROM clause of a select. (INSERT and UPDATE statements are the same as a SELECT with only a single table in the FROM clause.) For example, if the SQL is this:
//			SELECT * FROM t1, t2, t3 WHERE ...;
//	Then the code generated is conceptually like the following:
//			foreach row1 in t1 do       \    Code generated
//			  foreach row2 in t2 do      |-- by WhereBegin()
//				foreach row3 in t3 do   /
//				  ...
//				end                     \    Code generated
//			  end                        |-- by sqlite3WhereEnd()
//			end                         /
//	Note that the loops might not be nested in the order in which they appear in the FROM clause if a different order is better able to make use of indices. Note also that when the IN operator appears in the WHERE clause, it might result in additional nested loops for scanning through all values on the right-hand side of the IN.
//	There are Btree cursors associated with each table. t1 uses cursor number pTabList.a[0].iCursor. t2 uses the cursor pTabList.a[1].iCursor. And so forth. This routine generates code to open those VDBE cursors and sqlite3WhereEnd() generates the code to close them.
//	The code that WhereBegin() generates leaves the cursors named in pTabList pointing at their appropriate entries. The [...] code can use OP_Column and OP_Rowid opcodes on these cursors to extract data from the various tables of the loop.
//	If the WHERE clause is empty, the foreach loops must each scan their entire tables. Thus a three-way join is an O(N^3) operation. But if the tables have indices and there are terms in the WHERE clause that refer to those indices, a complete table scan can be avoided and the code will run much faster. Most of the work of this routine is checking to see if there are indices that can be used to speed up the loop.
//	Terms of the WHERE clause are also used to limit which rows actually make it to the "..." in the middle of the loop. After each "foreach", terms of the WHERE clause that use only terms in that loop and outer loops are evaluated and if false a jump is made around all subsequent inner loops (or around the "..." if the test occurs within the inner-most loop)
//
//	OUTER JOINS
//	An outer join of tables t1 and t2 is conceptally coded as follows:
//			foreach row1 in t1 do
//			  flag = 0
//			  foreach row2 in t2 do
//				start:
//				...
//				flag = 1
//			  end
//			  if flag==0 then
//				move the row2 cursor to a null row
//				goto start
//			  fi
//			end
//
//	ORDER BY CLAUSE PROCESSING
//	*ppOrderBy is a pointer to the ORDER BY clause of a SELECT statement, if there is one. If there is no ORDER BY clause or if this routine is called from an UPDATE or DELETE statement, then ppOrderBy is nil.
//	If an index can be used so that the natural output order of the table scan is correct for the ORDER BY clause, then that index is used and *ppOrderBy is set to nil. This is an optimization that prevents an unnecessary sort of the result set if an index appropriate for the ORDER BY clause already exists.
//	If the where clause loops cannot be arranged to provide the correct output order, then the *ppOrderBy is unchanged.
func (pParse *Parse) WhereBegin(tables *SrcList, WHERE *Expr, ORDER_BY **ExprList, DISTINCT *ExprList, Flags uint16) (pWInfo *WhereInfo) {

  uint16 Flags        /* One of the WHERE_* flags defined in sqliteInt.h */
){

  Bitmask notReady;          /* Cursors that are not yet positioned */
  struct SrcList_item *pTabItem;  /* A single entry from pTabList */
  WhereLevel *pLevel;             /* A single level in the pWInfo list */
  int iFrom;                      /* First unused FROM clause element */
  int andFlags;              /* AND-ed combination of all pWC.a[].wtFlags */

	//	The number of tables in the FROM clause is limited by the number of bits in a Bitmask 
	if len(tables) > BMS {
		pParse.SetErrorMsg("at most %v tables in a join", BMS)
		return
	}

	//	This function normally generates a nested loop for all tables in pTabList. But if the WHERE_ONETABLE_ONLY flag is set, then we should only generate code for the first table in pTabList and assume that any cursors associated with subsequent tables are uninitialized.
	nTabList		int
	if Flags & WHERE_ONETABLE_ONLY != 0 {
		nTabList = 1
	} else {
		nTabList = len(tables)
	}

	//	Allocate and initialize the WhereInfo structure that will become the return value. A single allocation is used to store the WhereInfo struct, the contents of WhereInfo.a[], the WhereClause structure and the WhereMaskSet structure. Since WhereClause contains an 8-byte field (type Bitmask) it must be aligned on an 8-byte boundary on some architectures. Hence the ROUND(x, 8) below.
	db := pParse.db
	v := pParse.pVdbe

	pWInfo = &WhereInfo{ nLevel: nTabList, pParse: pParse, pTabList: tables, iBreak: v.MakeLabel(), Flags: Flags, savedNQueryLoop: pParse.nQueryLoop }

	pMaskSet := new(WhereMaskSet)
	pWC := &WhereClause{ pParse: pParse, WhereMaskSet: pMaskSet, Flags: Flags }
	pWInfo.pWC = pWC

	//	Split the WHERE clause into separate subexpressions where each subexpression is separated by an AND operator.
	sqlite3ExprCodeConstants(pParse, WHERE)
	pWC.Split(WHERE, TK_AND)
    
	//	Special case: a WHERE clause that is constant. Evaluate the expression and either jump over all of the code or fall thru.
	if WHERE != nil && (nTabList == 0 || sqlite3ExprIsConstantNotJoin(WHERE)) {
		sqlite3ExprIfFalse(pParse, WHERE, pWInfo.iBreak, SQLITE_JUMPIFNULL)
		WHERE = nil
	}

	//	Assign a bit from the bitmask to every term in the FROM clause.
	//	When assigning bitmask values to FROM clause cursors, it must be the case that if X is the bitmask for the N-th FROM clause term then the bitmask for all FROM clause terms to the left of the N-th term is (X-1). An expression from the ON clause of a LEFT JOIN can use its Expr.iRightJoinTable value to find the bitmask of the right table of the join. Subtracting one from the right table bitmask gives a bitmask for all tables to the left of the join. Knowing the bitmask for all tables to the left of a left join is important.  Ticket #3015.
	//	Configure the WhereClause.Bitmask variable so that bits that correspond to virtual table cursors are set. This is used to selectively disable the OR-to-IN transformation in exprAnalyzeOrTerm(). It is not helpful with virtual tables.
	//	Note that bitmasks are created for all tables in pTabList, not just the first nTabList tables.  nTabList is normally equal to len(pTabList) but might be shortened to 1 if the WHERE_ONETABLE_ONLY flag is set.
	assert( pWC.Bitmask == 0 && pMaskSet.n == 0 )
	for i, table := range tables {
		createMask(pMaskSet, table.iCursor)
		if t := table.pTab; t.IsVirtual() {
			pWC.Bitmask |= Bitmask(1) << i
		}
	}

	//	Analyze all of the subexpressions.  Note that exprAnalyze() might add new virtual terms onto the end of the WHERE clause. We do not want to analyze these virtual terms, so start analyzing at the end and work forward so that the added virtual terms are never processed.
	exprAnalyzeAll(tables, pWC)
	if db.mallocFailed {
		goto whereBeginError
	}

	//	Check if the DISTINCT qualifier, if there is one, is redundant. If it is, then set pDistinct to NULL and WhereInfo.eDistinct to WHERE_DISTINCT_UNIQUE to tell the caller to ignore the DISTINCT.
	if DISTINCT != nil && pParse.isDistinctRedundant(tables, pWC, pDistinct) {
		DISTINCT = nil
		pWInfo.eDistinct = WHERE_DISTINCT_UNIQUE
	}

	//	Choose the best index to use for each table in the FROM clause.
	//	This loop fills in the following fields:
	//			pWInfo.a[].pIdx      The index to use for this level of the loop.
	//			pWInfo.a[].wsFlags   WHERE_xxx flags associated with pIdx
	//			pWInfo.a[].nEq       The number of == and IN constraints
	//			pWInfo.a[].iFrom     Which term of the FROM clause is being coded
	//			pWInfo.a[].iTabCur   The VDBE cursor for the database table
	//			pWInfo.a[].iIdxCur   The VDBE cursor for the index
	//			pWInfo.a[].pTerm     When wsFlags==WO_OR, the OR-clause term
	//	This loop also figures out the nesting order of tables in the FROM clause.
	notReady = ~Bitmask(0)
	andFlags = ~0
	WHERETRACE( "*** Optimizer Start ***\n" )
	for i := From = 0, pLevel = pWInfo.a; i < nTabList; i++, pLevel++ {
		Index *pIdx;                /* Index for FROM table at pTabItem */
		int j;                      /* For looping over FROM tables */
		int bestJ = -1;             /* The value of j */
		Bitmask m;                  /* Bitmask value for j or bestJ */
		int isOptimal;              /* Iterator for optimal/non-optimal search */
		int nUnconstrained;         /* Number tables without INDEXED BY */
		Bitmask notIndexed;         /* Mask of tables that cannot use an index */

		bestPlan := &WhereCost{ rCost: BIG_DOUBLE }					//	Most efficient plan seen so far
		WHERETRACE( "*** Begin search for loop %d ***\n", i)

		//	Loop through the remaining entries in the FROM clause to find the next nested loop. The loop tests all FROM clause entries either once or twice. 
		//	The first test is always performed if there are two or more entries remaining and never performed if there is only one FROM clause entry to choose from. The first test looks for an "optimal" scan. In this context an optimal scan is one that uses the same strategy for the given FROM clause entry as would be selected if the entry were used as the innermost nested loop. In other words, a table is chosen such that the cost of running that table cannot be reduced by waiting for other tables to run first. This "optimal" test works by first assuming that the FROM clause is on the inner loop and finding its query plan, then checking to see if that query plan uses any other FROM clause terms that are notReady. If no notReady terms are used then the "optimal" query plan works.
		//	Note that the WhereCost.nRow parameter for an optimal scan might not be as small as it would be if the table really were the innermost join. The nRow value can be reduced by WHERE clause constraints that do not use indices. But this nRow reduction only happens if the table really is the innermost join.  
		//	The second loop iteration is only performed if no optimal scan strategies were found by the first iteration. This second iteration is used to search for the lowest cost scan overall.
		//	Previous versions of SQLite performed only the second iteration - the next outermost loop was always that with the lowest overall cost. However, this meant that SQLite could select the wrong plan for scripts such as the following:
		//			CREATE TABLE t1(a, b);
		//			CREATE TABLE t2(c, d);
		//			SELECT * FROM t2, t1 WHERE t2.rowid = t1.a;
		//	The best strategy is to iterate through table t1 first. However it is not possible to determine this with a simple greedy algorithm. Since the cost of a linear scan through table t2 is the same as the cost of a linear scan through table t1, a simple greedy algorithm may choose to use t2 for the outer loop, which is a much costlier approach.
		nUnconstrained = 0
		notIndexed = 0
		for isOptimal := (iFrom < nTabList - 1); isOptimal >= 0 && bestJ < 0; isOptimal-- {
			Bitmask mask;             /* Mask of tables not yet ready */
			for j := iFrom, pTabItem := &tables.a[j]; j < nTabList; j++, pTabItem++ {
				WhereCost sCost;     /* Cost information from best[Virtual]Index() */
  
				doNotReorder :=  (pTabItem.jointype & (JT_LEFT|JT_CROSS)) != 0
				if j != iFrom && doNotReorder {
					break
				}
				m = getMask(pMaskSet, pTabItem.iCursor)
				if (m & notReady) == 0 {
					if j == iFrom {
						iFrom++
					}
					continue
				}
				if isOptimal {
					mask = m
				} else {
					mask = notReady
				}
				if pTabItem.Indices == nil {
					nUnconstrained++
				}
  
				WHERETRACE( "=== trying table %d with isOptimal=%d ===\n", j, isOptimal )
				assert( pTabItem.pTab )
				if i == 0 {
					if pTabItem.pTab.IsVirtual() {
						sqlite3_index_info **pp = &pWInfo.a[j].pIdxInfo
						bestVirtualIndex(pParse, pWC, pTabItem, mask, notReady, ORDER_BY, &sCost, pp)
					} else  {
						bestBtreeIndex(pParse, pWC, pTabItem, mask, notReady, ORDER_BY, DISTINCT, &sCost)
					}
				} else {
					if pTabItem.pTab.IsVirtual() {
						sqlite3_index_info **pp = &pWInfo.a[j].pIdxInfo
						bestVirtualIndex(pParse, pWC, pTabItem, mask, notReady, nil, &sCost, pp)
					} else  {
						bestBtreeIndex(pParse, pWC, pTabItem, mask, notReady, nil, nil, &sCost)
					}
				}
				assert( isOptimal || sCost.used & notReady == 0 )

				//	If an INDEXED BY clause is present, then the plan must use that index if it uses any index at all
				assert( pTabItem.Indices == nil  || sCost.plan.wsFlags & WHERE_NOT_FULLSCAN == 0 || sCost.plan.u.pIdx == pTabItem.Indices )
				if isOptimal && sCost.plan.wsFlags & WHERE_NOT_FULLSCAN == 0 {
					notIndexed |= m
				}

				//	Conditions under which this table becomes the best so far:
				//		(1) The table must not depend on other tables that have not yet run.
				//		(2) A full-table-scan plan cannot supercede indexed plan unless the full-table-scan is an "optimal" plan as defined above.
				//		(3) All tables have an INDEXED BY clause or this table lacks an INDEXED BY clause or this table uses the specific index specified by its INDEXED BY clause. This rule ensures that a best-so-far is always selected even if an impossible combination of INDEXED BY clauses are given. The error will be detected and relayed back to the application later.
				//		(4) The plan cost must be lower than prior plans or else the cost must be the same and the number of rows must be lower.
				if (sCost.used & notReady)==0                       /* (1) */
							&& (bestJ < 0 || (notIndexed & m)!=0               /* (2) */
									|| (bestPlan.plan.wsFlags & WHERE_NOT_FULLSCAN) == 0 || (sCost.plan.wsFlags & WHERE_NOT_FULLSCAN) != 0)
							&& (nUnconstrained == 0 || pTabItem.Indices == nil   /* (3) */
									|| (sCost.plan.wsFlags & WHERE_NOT_FULLSCAN) != 0)
							&& (bestJ < 0 || sCost.rCost < bestPlan.rCost      /* (4) */
									|| (sCost.rCost <= bestPlan.rCost && sCost.plan.nRow < bestPlan.plan.nRow)) {
					WHERETRACE( "=== table %d is best so far with cost=%g and nRow=%g\n", j, sCost.rCost, sCost.plan.nRow )
					bestPlan = sCost
					bestJ = j
				}
				if doNotReorder {
					break;
				}
			}
		}
		assert( bestJ >= 0 )
		assert( notReady & getMask(pMaskSet, tables.a[bestJ].iCursor) )
		WHERETRACE( "*** Optimizer selects table %d for loop %d with cost=%g and nRow=%g\n", bestJ, pLevel - pWInfo.a, bestPlan.rCost, bestPlan.plan.nRow )
		if bestPlan.plan.wsFlags & WHERE_ORDERBY != 0 && ORDER_BY != nil {
			ORDER_BY = nil
		}
		if bestPlan.plan.wsFlags & WHERE_DISTINCT != 0 {
			assert( pWInfo.eDistinct == 0 )
			pWInfo.eDistinct = WHERE_DISTINCT_ORDERED
		}
		andFlags &= bestPlan.plan.wsFlags
		pLevel.plan = bestPlan.plan
		if bestPlan.plan.wsFlags & (WHERE_INDEXED | WHERE_TEMP_INDEX) {
			pLevel.iIdxCur = pParse.nTab++
		} else {
			pLevel.iIdxCur = -1
		}
		notReady &= ~getMask(pMaskSet, tables.a[bestJ].iCursor)
		pLevel.iFrom = byte(bestJ)
		if bestPlan.plan.nRow >= float64(1) {
			pParse.nQueryLoop *= bestPlan.plan.nRow
		}

		//	Check that if the table scanned by this loop iteration had an INDEXED BY clause attached to it, that the named index is being used for the scan. If not, then query compilation has failed. Return an error.
		if pIdx = tables.a[bestJ].Indices; pIdx != nil {
			if bestPlan.plan.wsFlags & WHERE_INDEXED == 0 {
				pParse.SetErrorMsg("cannot use index: %v", pIdx.Name)
				goto whereBeginError
			} else {
				//	If an INDEXED BY clause is used, the bestIndex() function is guaranteed to find the index specified in the INDEXED BY clause if it find an index at all.
				assert( bestPlan.plan.u.pIdx == pIdx )
			}
		}
	}
	WHERETRACE( "*** Optimizer Finished ***\n" )
	if pParse.nErr || db.mallocFailed {
		goto whereBeginError
	}

	//	If the total query only selects a single row, then the ORDER BY clause is irrelevant.
	if andFlags & WHERE_UNIQUE != 0 && ORDER_BY != nil {
		ORDER_BY = nil
	}

	//	If the caller is an UPDATE or DELETE statement that is requesting to use a one-pass algorithm, determine if this is appropriate. The one-pass algorithm only works if the WHERE clause constraints the statement to update a single row.
	assert( Flags & WHERE_ONEPASS_DESIRED == 0 || pWInfo.nLevel == 1 )
	if Flags & WHERE_ONEPASS_DESIRED != 0 && andFlags & WHERE_UNIQUE != 0 {
		pWInfo.okOnePass = true
		pWInfo.a[0].plan.wsFlags &= ~WHERE_IDX_ONLY
	}

	//	Open all tables in the pTabList and any indices selected for searching those tables.
	pParse.CodeVerifySchema(-1)					//	Insert the cookie verifier Goto
	notReady = ~Bitmask(0)
	pWInfo.nRowOut = float64(1)
	for i := 0, pLevel = pWInfo.a; i < nTabList; i++, pLevel++){
		pTabItem = &tables.a[pLevel.iFrom]
		pLevel.iTabCur = pTabItem.iCursor
		pWInfo.nRowOut *= pLevel.plan.nRow
		table := pTabItem.pTab
		iDb := db.SchemaToIndex(table.Schema)
		switch {
		case table.tabFlags & TF_Ephemeral != 0 || table.Select:
			/* Do nothing */
		case pLevel.plan.wsFlags & WHERE_VIRTUALTABLE != 0:
			sqlite3VdbeAddOp4(v, OP_VOpen, pTabItem.iCursor, 0, 0, sqlite3GetVTable(db, table), P4_VTAB)
		case pLevel.plan.wsFlags & WHERE_IDX_ONLY == 0 && Flags & WHERE_OMIT_OPEN_CLOSE == 0:
			if pWInfo.okOnePass {
				pParse.OpenTable(table, pTabItem.iCursor, iDb, OP_OpenWrite)
			} else {
				pParse.OpenTable(table, pTabItem.iCursor, iDb, OP_OpenRead)
			}
			if !pWInfo.okOnePass && table.nCol < BMS {
				n := 0
				for b := pTabItem.colUsed; b != 0; b = b >> 1, n++ {}
				sqlite3VdbeChangeP4(v, v.CurrentAddr() - 1, SQLITE_INT_TO_PTR(n), P4_INT32)
				assert( n <= table.nCol )
			}
		default:
			pParse.TableLock(iDb, table.tnum, table.Name, false)
		}

		switch {
		case pLevel.plan.wsFlags & WHERE_TEMP_INDEX != 0:
			constructAutomaticIndex(pParse, pWC, pTabItem, notReady, pLevel)
		case pLevel.plan.wsFlags & WHERE_INDEXED != 0:
			pIx := pLevel.plan.u.pIdx
			pKey := pParse.IndexKeyinfo(pIx)
			iIdxCur := pLevel.iIdxCur
			assert( pIx.Schema == table.Schema )
			assert( iIdxCur >= 0 )
			sqlite3VdbeAddOp4(v, OP_OpenRead, iIdxCur, pIx.tnum, iDb, (char*)pKey, P4_KEYINFO_HANDOFF)
			v.Comment(pIx.Name)
		}
		pParse.CodeVerifySchema(iDb)
		notReady &= ~getMask(pWC.WhereMaskSet, pTabItem.iCursor)
	}
	pWInfo.iTop = v.CurrentAddr()
	if db.mallocFailed {
		goto whereBeginError
	}

	//	Generate the code to do the search. Each iteration of the for loop below generates code for a single nested loop of the VM program.
	notReady = ~Bitmask(0)
	for i := 0; i < nTabList; i++ {
		pLevel = &pWInfo.a[i]
		explainOneScan(pParse, tables, pLevel, i, pLevel.iFrom, Flags)
		notReady = codeOneLoopStart(pWInfo, i, Flags, notReady)
		pWInfo.iContinue = pLevel.addrCont
	}

	//	Record the continuation address in the WhereInfo structure. Then clean up and return.
	return pWInfo

  /* Jump here if malloc fails */
whereBeginError:
	if pWInfo != nil {
		pParse.nQueryLoop = pWInfo.savedNQueryLoop
		whereInfoFree(db, pWInfo)
	}
	return
}

//	Generate the end of the WHERE loop. See comments on WhereBegin() for additional information.
 void sqlite3WhereEnd(WhereInfo *pWInfo){
  Parse *pParse = pWInfo.pParse;
  Vdbe *v = pParse.pVdbe;
  int i;
  WhereLevel *pLevel;

	pTabList := pWInfo.pTabList
	db := pParse.db

	//	Generate loop termination code.
	sqlite3ExprCacheClean(pParse)
	for i := pWInfo.nLevel - 1; i >= 0; i-- {
		pLevel = &pWInfo.a[i]
		v.ResolveLabel(pLevel.addrCont)
		if pLevel.op != OP_Noop {
			v.AddOp2(pLevel.op, pLevel.p1, pLevel.p2)
			v.ChangeP5(pLevel.p5)
		}
		if pLevel.plan.wsFlags & WHERE_IN_ABLE && pLevel.u.in.nIn > 0 {
			struct InLoop *pIn
			v.ResolveLabel(pLevel.addrNxt)
			for j, , pLevel.u.in.aInLoop[j - 1]; j > 0; j-- {
				pIn := pLevel.u.in.nIn
				v.JumpHere(pIn.addrInTop + 1)
				v.AddOp2(OP_Next, pIn.iCur, pIn.addrInTop)
				v.JumpHere(pIn.addrInTop - 1)
				pIn--
			}
			pLevel.u.in.aInLoop = nil
		}
		v.ResolveLabel(pLevel.addrBrk)
		if pLevel.iLeftJoin {
			int addr
			addr = v.AddOp1(OP_IfPos, pLevel.iLeftJoin)
			assert( pLevel.plan.wsFlags & WHERE_IDX_ONLY == 0 || pLevel.plan.wsFlags & WHERE_INDEXED != 0 )
			if pLevel.plan.wsFlags & WHERE_IDX_ONLY == 0 {
				v.AddOp1(OP_NullRow, pTabList.a[i].iCursor);
			}
			if pLevel.iIdxCur >= 0 {
				v.AddOp1(OP_NullRow, pLevel.iIdxCur)
			}
			if pLevel.op == OP_Return {
				v.AddOp2(OP_Gosub, pLevel.p1, pLevel.addrFirst)
			} else {
				v.AddOp2(OP_Goto, 0, pLevel.addrFirst);
			}
			v.JumpHere(addr)
		}
	}

	//	The "break" point is here, just past the end of the outer loop. Set it.
	v.ResolveLabel(pWInfo.iBreak)

	//	Close all of the cursors that were opened by WhereBegin().
	assert( pWInfo.nLevel == 1 || pWInfo.nLevel == len(pTabList) )
	for i, pLevel := 0, pWInfo.a; i < pWInfo.nLevel; i++, pLevel++) {
		pTabItem := pTabList[pLevel.iFrom]
		pTab := pTabItem.pTab
		if (pTab.tabFlags & TF_Ephemeral) == 0 && pTab.Select == 0 && (pWInfo.Flags & WHERE_OMIT_OPEN_CLOSE) == 0) {
			int ws = pLevel.plan.wsFlags
			if !pWInfo.okOnePass && (ws & WHERE_IDX_ONLY) == 0  {
				v.AddOp1(OP_Close, pTabItem.iCursor)
			}
			if (ws & WHERE_INDEXED) != 0 && (ws & WHERE_TEMP_INDEX) == 0 {
				v.AddOp1(OP_Close, pLevel.iIdxCur)
			}
		}

		//	If this scan uses an index, make code substitutions to read data from the index in preference to the table. Sometimes, this means the table need never be read from. This is a performance boost, as the vdbe level waits until the table is read before actually seeking the table cursor to the record corresponding to the current position in the index.
		//	Calls to the code generator in between WhereBegin() and sqlite3WhereEnd will have created code that references the table directly. This loop scans all that code looking for opcodes that reference the table and converts them into opcodes that reference the index.
		if (pLevel.plan.wsFlags & WHERE_INDEXED) != 0 && !db.mallocFailed {
			pIdx := pLevel.plan.u.pIdx
			pOp := sqlite3VdbeGetOp(v, pWInfo.iTop)
			last := v.CurrentAddr()
			for k := pWInfo.iTop; k < last; k++, pOp++ {
				if pOp.p1 != pLevel.iTabCur {
					continue
				}
				if pOp.opcode == OP_Column {
					for j, column := range pIdx.Columns {
						if pOp.p2 == column {
							pOp.p2 = j
							pOp.p1 = pLevel.iIdxCur
							break
						}
					}
					assert( (pLevel.plan.wsFlags & WHERE_IDX_ONLY) == 0 || j < len(pIdx.Columns) )
				} else if pOp.opcode == OP_Rowid {
					pOp.p1 = pLevel.iIdxCur
					pOp.opcode = OP_IdxRowid
				}
			}
		}
	}

	//	Final cleanup
	pParse.nQueryLoop = pWInfo.savedNQueryLoop
	whereInfoFree(db, pWInfo)
	return
}