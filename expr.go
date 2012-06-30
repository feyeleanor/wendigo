import "crypto/rand"


/* This file contains routines used for analyzing expressions and
** for generating VDBE code that evaluates expressions in SQLite.
*/

/*
** Return the 'affinity' of the expression pExpr if any.
**
** If pExpr is a column, a reference to a column via an 'AS' alias,
** or a sub-select with a column as the return value, then the 
** affinity of that column is returned. Otherwise, 0x00 is returned,
** indicating no affinity for the expression.
**
** i.e. the WHERE clause expresssions in the following statements all
** have an affinity:
**
** CREATE TABLE t1(a);
** SELECT * FROM t1 WHERE a;
** SELECT a AS b FROM t1 WHERE b;
** SELECT * FROM t1 WHERE (select a from t1);
*/
 char sqlite3ExprAffinity(Expr *pExpr){
  int op = pExpr.op;
  if( op==TK_SELECT ){
    assert( pExpr.flags&EP_xIsSelect );
    return sqlite3ExprAffinity(pExpr.x.Select.pEList.a[0].Expr);
  }
#ifndef SQLITE_OMIT_CAST
  if( op==TK_CAST ){
    assert( !pExpr.HasProperty(EP_IntValue) );
    return sqlite3AffinityType(pExpr.Token);
  }
#endif
  if( (op==TK_AGG_COLUMN || op==TK_COLUMN || op==TK_REGISTER) 
   && pExpr.pTab!=0
  ){
    /* op==TK_REGISTER && pExpr.pTab!=0 happens when pExpr was originally
    ** a TK_COLUMN but was previously evaluated and cached in a register */
    int j = pExpr.iColumn;
    if( j<0 ) return SQLITE_AFF_INTEGER;
    assert( pExpr.pTab && j<pExpr.pTab.nCol );
    return pExpr.pTab.Columns[j].affinity;
  }
  return pExpr.affinity;
}

/*
** Set the explicit collating sequence for an expression to the
** collating sequence supplied in the second argument.
*/
 Expr *sqlite3ExprSetColl(Expr *pExpr, CollSeq *pColl){
  if( pExpr && pColl ){
    pExpr.pColl = pColl;
    pExpr.flags |= EP_ExpCollate;
  }
  return pExpr;
}

/*
** Set the collating sequence for expression pExpr to be the collating
** sequence named by pToken.   Return a pointer to the revised expression.
** The collating sequence is marked as "explicit" using the EP_ExpCollate
** flag.  An explicit collating sequence will override implicit
** collating sequences.
*/
 Expr *sqlite3ExprSetCollByToken(Parse *pParse, Expr *pExpr, Token *pCollName){
  char *zColl = 0;            /* Dequoted name of collation sequence */
  CollSeq *pColl;
  sqlite3 *db = pParse.db;
  zColl = Dequote(pCollName)
  pColl = pParse.LocateCollSeq(zColl)
  sqlite3ExprSetColl(pExpr, pColl);
  zColl = nil
  return pExpr;
}

/*
** Return the default collation sequence for the expression pExpr. If
** there is no default collation type, return 0.
*/
 CollSeq *sqlite3ExprCollSeq(Parse *pParse, Expr *pExpr){
  CollSeq *pColl = 0;
  Expr *p = pExpr;
  while( p ){
    int op;
    pColl = p.pColl;
    if( pColl ) break;
    op = p.op;
    if( p.pTab!=0 && (
        op==TK_AGG_COLUMN || op==TK_COLUMN || op==TK_REGISTER || op==TK_TRIGGER
    )){
      /* op==TK_REGISTER && p.pTab!=0 happens when pExpr was originally
      ** a TK_COLUMN but was previously evaluated and cached in a register */
      const char *zColl;
      int j = p.iColumn;
      if( j>=0 ){
        sqlite3 *db = pParse.db;
        zColl = p.pTab.Columns[j].zColl;
        pColl = sqlite3FindCollSeq(db, db.Encoding(), zColl, 0);
        pExpr.pColl = pColl;
      }
      break;
    }
    if( op!=TK_CAST && op!=TK_UPLUS ){
      break;
    }
    p = p.pLeft;
  }
  if( sqlite3CheckCollSeq(pParse, pColl) ){ 
    pColl = 0;
  }
  return pColl;
}

/*
** pExpr is an operand of a comparison operator.  aff2 is the
** type affinity of the other operand.  This routine returns the
** type affinity that should be used for the comparison operator.
*/
 char sqlite3CompareAffinity(Expr *pExpr, char aff2){
  char aff1 = sqlite3ExprAffinity(pExpr);
  if( aff1 && aff2 ){
    /* Both sides of the comparison are columns. If one has numeric
    ** affinity, use that. Otherwise use no affinity.
    */
    if( sqlite3IsNumericAffinity(aff1) || sqlite3IsNumericAffinity(aff2) ){
      return SQLITE_AFF_NUMERIC;
    }else{
      return SQLITE_AFF_NONE;
    }
  }else if( !aff1 && !aff2 ){
    /* Neither side of the comparison is a column.  Compare the
    ** results directly.
    */
    return SQLITE_AFF_NONE;
  }else{
    /* One side is a column, the other is not. Use the columns affinity. */
    assert( aff1==0 || aff2==0 );
    return (aff1 + aff2);
  }
}

/*
** pExpr is a comparison operator.  Return the type affinity that should
** be applied to both operands prior to doing the comparison.
*/
static char comparisonAffinity(Expr *pExpr){
  char aff;
  assert( pExpr.op==TK_EQ || pExpr.op==TK_IN || pExpr.op==TK_LT ||
          pExpr.op==TK_GT || pExpr.op==TK_GE || pExpr.op==TK_LE ||
          pExpr.op==TK_NE || pExpr.op==TK_IS || pExpr.op==TK_ISNOT );
  assert( pExpr.pLeft );
  aff = sqlite3ExprAffinity(pExpr.pLeft);
  if( pExpr.pRight ){
    aff = sqlite3CompareAffinity(pExpr.pRight, aff);
  }else if( pExpr.HasProperty(EP_xIsSelect) ){
    aff = sqlite3CompareAffinity(pExpr.x.Select.pEList.a[0].Expr, aff);
  }else if( !aff ){
    aff = SQLITE_AFF_NONE;
  }
  return aff;
}

/*
** pExpr is a comparison expression, eg. '=', '<', IN(...) etc.
** idx_affinity is the affinity of an indexed column. Return true
** if the index with affinity idx_affinity may be used to implement
** the comparison in pExpr.
*/
 int sqlite3IndexAffinityOk(Expr *pExpr, char idx_affinity){
  char aff = comparisonAffinity(pExpr);
  switch( aff ){
    case SQLITE_AFF_NONE:
      return 1;
    case SQLITE_AFF_TEXT:
      return idx_affinity==SQLITE_AFF_TEXT;
    default:
      return sqlite3IsNumericAffinity(idx_affinity);
  }
}

/*
** Return the P5 value that should be used for a binary comparison
** opcode (OP_Eq, OP_Ge etc.) used to compare pExpr1 and pExpr2.
*/
static byte binaryCompareP5(Expr *pExpr1, Expr *pExpr2, int jumpIfNull){
  byte aff = (char)sqlite3ExprAffinity(pExpr2);
  aff = (byte)sqlite3CompareAffinity(pExpr1, aff) | (byte)jumpIfNull;
  return aff;
}

/*
** Return a pointer to the collation sequence that should be used by
** a binary comparison operator comparing pLeft and pRight.
**
** If the left hand expression has a collating sequence type, then it is
** used. Otherwise the collation sequence for the right hand expression
** is used, or the default (BINARY) if neither expression has a collating
** type.
**
** Argument pRight (but not pLeft) may be a null pointer. In this case,
** it is not considered.
*/
 CollSeq *sqlite3BinaryCompareCollSeq(
  Parse *pParse, 
  Expr *pLeft, 
  Expr *pRight
){
  CollSeq *pColl;
  assert( pLeft );
  if( pLeft.flags & EP_ExpCollate ){
    assert( pLeft.pColl );
    pColl = pLeft.pColl;
  }else if( pRight && pRight.flags & EP_ExpCollate ){
    assert( pRight.pColl );
    pColl = pRight.pColl;
  }else{
    pColl = sqlite3ExprCollSeq(pParse, pLeft);
    if( !pColl ){
      pColl = sqlite3ExprCollSeq(pParse, pRight);
    }
  }
  return pColl;
}

/*
** Generate code for a comparison operator.
*/
static int codeCompare(
  Parse *pParse,    /* The parsing (and code generating) context */
  Expr *pLeft,      /* The left operand */
  Expr *pRight,     /* The right operand */
  int opcode,       /* The comparison opcode */
  int in1, int in2, /* Register holding operands */
  int dest,         /* Jump here if true.  */
  int jumpIfNull    /* If true, jump if either operand is NULL */
){
  int p5;
  int addr;
  CollSeq *p4;

  p4 = sqlite3BinaryCompareCollSeq(pParse, pLeft, pRight);
  p5 = binaryCompareP5(pLeft, pRight, jumpIfNull);
  addr = sqlite3VdbeAddOp4(pParse.pVdbe, opcode, in2, dest, in1, (void*)p4, P4_COLLSEQ);
  pParse.pVdbe.ChangeP5(byte(p5))
  return addr;
}

  #define exprSetHeight(y)

/*
** This routine is the core allocator for Expr nodes.
**
** Construct a new expression node and return a pointer to it.  Memory
** for this node and for the pToken argument is a single allocation
** obtained from sqlite3DbMalloc().  The calling function
** is responsible for making sure the node eventually gets freed.
**
** If dequote is true, then the token (if it exists) is dequoted.
** If dequote is false, no dequoting is performance.  The deQuote
** parameter is ignored if pToken is NULL or if the token does not
** appear to be quoted.  If the quotes were of the form "..." (double-quotes)
** then the EP_DblQuoted flag is set on the expression node.
**
** Special case:  If op==TK_INTEGER and pToken points to a string that
** can be translated into a 32-bit integer, then the token is not
** stored in Token.  Instead, the integer values is written
** into Value and the EP_IntValue flag is set.  No extra storage
** is allocated to hold the integer text and the dequote flag is ignored.
*/
 Expr *sqlite3ExprAlloc(
  sqlite3 *db,            /* Handle for sqlite3DbMallocZero() (may be null) */
  int op,                 /* Expression opcode */
  const Token *pToken,    /* Token argument.  Might be NULL */
  int dequote             /* True to dequote */
){
  Expr *pNew;
  int nExtra = 0;
  int Value = 0;

  if( pToken ){
    if( op!=TK_INTEGER || pToken.z==0
          || sqlite3GetInt32(pToken.z, &Value)==0 ){
      nExtra = pToken.n+1;
      assert( Value>=0 );
    }
  }
  pNew = sqlite3DbMallocZero(db, sizeof(Expr)+nExtra);
  if( pNew ){
    pNew.op = (byte)op;
    pNew.iAgg = -1;
    if( pToken ){
      if( nExtra==0 ){
        pNew.flags |= EP_IntValue;
        pNew.Value = Value;
      }else{
        int c;
        pNew.Token = (char*)&pNew[1];
        assert( pToken.z!=0 || pToken.n==0 );
        if( pToken.n ) memcpy(pNew.Token, pToken.z, pToken.n);
        pNew.Token[pToken.n] = 0;
        if( dequote && nExtra>=3 && ((c = pToken.z[0])=='\'' || c=='"' || c=='[' || c=='`') ){
          pNew.Token = Dequote(pNew.Token)
          if( c=='"' ) pNew.flags |= EP_DblQuoted;
        }
      }
    }
  }
  return pNew;
}

//	Allocate a new expression node from a zero-terminated token that has already been dequoted.
func (db *sqlite3) Expr(opcode int, token string) *Expr {
	return sqlite3ExprAlloc(db, opcode, token, 0)
}

//	Attach subtrees pLeft and pRight to the Expr node pRoot.
//	If pRoot == nil that means that a memory allocation error has occurred. In that case, delete the subtrees pLeft and pRight.
func (db *sqlite3) ExprAttachSubtrees(root, left, right *Expr) {
 void sqlite3ExprAttachSubtrees(
  sqlite3 *db,
  Expr *pRoot,
  Expr *pLeft,
  Expr *pRight
){
	if root == nil {
		assert( db.mallocFailed )
		db.ExprDelete(left)
		db.ExprDelete(right)
	} else {
		if right != nil {
			root.pRight = right
			if right.flags & EP_ExpCollate != 0 {
				root.flags |= EP_ExpCollate
				root.pColl = right.pColl
			}
		}
		if left != nil {
			root.pLeft = left
			if left.flags & EP_ExpCollate != nil {
				root.flags |= EP_ExpCollate
				root.pColl = left.pColl
			}
		}
	}
}

//	Allocate an Expr node which joins as many as two subtrees.
//	One or both of the subtrees can be nil. Return a pointer to the new Expr node. Or, if an OOM error occurs, set pParse.db.mallocFailed, free the subtrees and return nil.
func (pParse *Parse) Expr(opcode int, left, right *Expr, argument string) (e *Expr)
	if opcode == TK_AND && left != nil && right != nil {
		//	Take advantage of short-circuit false optimization for AND
		e = pParse.db.ExprAnd(left, right)
	} else {
		e = sqlite3ExprAlloc(pParse.db, opcode, argument, 1)
		sqlite3ExprAttachSubtrees(pParse.db, e, left, right)
	}
	return
}

//	Return 1 if an expression must be FALSE in all cases and 0 if the expression might be true. This is an optimization. If is OK to return 0 here even if the expression really is always false (a false negative). But it is a bug to return 1 if the expression might be true in some rare circumstances (a false positive.)
//	Note that if the expression is part of conditional for a LEFT JOIN, then we cannot determine at compile-time whether or not is it true or false, so always return 0.
func (p *Expr) AlwaysFalse() (b bool) {
	if !p.HasProperty(EP_FromJoin) || sqlite3ExprIsInteger(p, &v) {
		b = true
	}
	return
}

//	Join two expressions using an AND operator. If either expression is nil, then just return the other expression.
//	If one side or the other of the AND is known to be false, then instead of returning an AND expression, just return a constant expression with a value of false.
func (db *sqlite3) ExprAnd(left, right *Expr) (e *Expr) {
	switch {
	case left == nil:
		e = right
	case right == nil:
		e = left
	case left.AlwaysFalse() || right.AlwaysFalse():
		db.ExprDelete(left)
		db.ExprDelete(right)
		e = sqlite3ExprAlloc(db, TK_INTEGER, &sqlite3IntTokens[0], 0)
	default:
		e = sqlite3ExprAlloc(db, TK_AND, 0, 0)
		sqlite3ExprAttachSubtrees(db, pNew, left, right)
	}
	return
}

//	Construct a new expression node for a function with multiple arguments.
func (pParse *Parse) ExprFunction(pList *ExprList, pToken *Token) (pNew *Expr) {
	db := pParse.db
	assert( pToken != nil )
	pNew = &Expr{ pList: pList }
	assert( !pNew.HasProperty(EP_xIsSelect) )
	return
}

//	Assign a variable number to an expression that encodes a wildcard in the original SQL statement.  
//	Wildcards consisting of a single "?" are assigned the next sequential variable number.
//	Wildcards of the form "?nnn" are assigned the number "nnn".  We make sure "nnn" is not too be to avoid a denial of service attack when the SQL statement comes from an external source.
//	Wildcards of the form ":aaa", "@aaa", or "$aaa" are assigned the same number as the previous instance of the same wildcard. Or if this is the first instance of the wildcard, the next sequenial variable number is assigned.
void sqlite3ExprAssignVarNumber(Parse *pParse, Expr *pExpr){
  sqlite3 *db = pParse.db;
  const char *z;

  if( pExpr==0 ) return;
  assert( !pExpr.HasProperty(EP_IntValue) )
  z = pExpr.Token;
  assert( z!=0 );
  assert( z[0]!=0 );
  if( z[1]==0 ){
    /* Wildcard of the form "?".  Assign the next variable number */
    assert( z[0]=='?' );
    pExpr.iColumn = (ynVar)(++pParse.nVar);
  }else{
    ynVar x = 0;
    uint32 n = sqlite3Strlen30(z);
    if( z[0]=='?' ){
      /* Wildcard of the form "?nnn".  Convert "nnn" to an integer and
      ** use it as the variable number */
      int64 i;
      int bOk = Atoint64(&z[1], &i, n-1, SQLITE_UTF8) == 0
      pExpr.iColumn = x = (ynVar)i;
      if( bOk==0 || i<1 ){
        pParse.SetErrorMsg("variable number must be ?1 or greater")
        x = 0;
      }
      if( i>pParse.nVar ){
        pParse.nVar = (int)i;
      }
    }else{
      /* Wildcards like ":aaa", "$aaa" or "@aaa".  Reuse the same variable
      ** number as the prior appearance of the same name, or if the name
      ** has never appeared before, reuse the same variable number
      */
      ynVar i;
      for(i=0; i<pParse.nzVar; i++){
        if( pParse.azVar[i] && memcmp(pParse.azVar[i],z,n+1)==0 ){
          pExpr.iColumn = x = (ynVar)i+1;
          break;
        }
      }
      if( x==0 ) x = pExpr.iColumn = (ynVar)(++pParse.nVar);
    }
    if( x>0 ){
      if( x>pParse.nzVar ){
        char **a;
        a = sqlite3DbRealloc(db, pParse.azVar, x*sizeof(a[0]));
        if( a==0 ) return;  /* Error reported through db.mallocFailed */
        pParse.azVar = a;
        memset(&a[pParse.nzVar], 0, (x-pParse.nzVar)*sizeof(a[0]));
        pParse.nzVar = x;
      }
      if( z[0]!='?' || pParse.azVar[x-1]==0 ){
        pParse.azVar[x-1] = sqlite3DbStrNDup(db, z, n);
      }
    }
  }
}

//	Recursively delete an expression tree.
func (db *sqlite3) ExprDelete(p *Expr) {
	if p != nil {
		//	Sanity check: Assert that the IntValue is non-negative if it exists
		assert( !p.HasProperty(EP_IntValue) || p.Value >= 0 )
		db.ExprDelete(p.pLeft)
		db.ExprDelete(p.pRight)
		if p.HasProperty(EP_xIsSelect) {
			sqlite3SelectDelete(db, p.x.Select)
		} else {
			db.ExprListDelete(p.pList)
		}
		*p = (*Expr)(nil)
	}
}

//	The following group of routines make deep copies of expressions, expression lists, ID lists, and select statements. The copies can be deleted (by being passed to their respective ...Delete() routines) without effecting the originals.
//	The expression list, ID, and source lists return by ExprList::Dup(), IdList::Dup(), and SrcList::Dup() can not be further expanded by subsequent calls to sqlite*ListAppend() routines.
//	Any tables that the SrcList might point to are not duplicated.
func (p *Expr) Dup() (pNew *Expr) {
	if p != nil {
		pNew = &Expr{
			op:					p.op,
			affinity:			p.affinity,
			flags:				p.flags,
			Value:				p.Value,
			Token:				p.Token,
			pLeft:				p.pLeft.Dup(),
			pRight:				p.pRight.Dup(),
			pColl:				p.pColl,
			iTable:				p.iTable,
			iColumn:			p.iColumn,
			iAgg:				p.iAgg,
			iRightJoinTable:	p.iRightJoinTable,
			op2:				p.op2,
			pAggInfo:			p.pAggInfo,
			pTab:				p.pTab,
		}

		if p.HasProperty(EP_xIsSelect) {
			pNew.pSelect = p.pSelect.Dup();
		} else {
			pNew.pList = p.pList.Dup();
		}
	}
	return
}

func (p *ExprList) Dup() (pNew *ExprList) {
	if p != nil {
		pNew = NewExprList(make([]ExprList_item, len(p.Items)))
		for i, item := range p.Items {
    		pOldExpr := item
			p.Item[i] = &ExprList_item{
				Expr:			pOldExpr.Dup(),
				Name:			pOldItem.Name,
				zSpan:			pOldItem.zSpan,
				sortOrder:		pOldItem.sortOrder,
				iOrderByCol:	pOldItem.iOrderByCol,
				iAlias:			pOldItem.iAlias,
			}
		}
	}
	return
}

func (p SrcList) Dup(flags int) (pNew SrcList) {
	if p != nil {
		return nil
	}
	if pNew = make(SrcList, len(p)); pNew != nil {
		for i := 0; i < p.nSrc; i++ {
			pNewItem = &pNew[i]
			pOldItem = &p[i]
	
			pNew[i] = &SrcList_item{ zDatabase: pOldItem.zDatabase, 
				Name: pOldItem.Name,
				zAlias: pOldItem.zAlias,
				jointype: pOldItem.jointype,
				iCursor: pOldItem.iCursor,
				addrFillSub: pOldItem.addrFillSub,
				regReturn: pOldItem.regReturn,
				isCorrelated: pOldItem.isCorrelated,
				zIndex: pOldItem.zIndex,
				notIndexed: pOldItem.notIndexed,
				pIndex: pOldItem.pIndex,
				pTab: pOldItem.pTab,
				Select: pOldItem.Select.Dup(),
				pOn: pOldItem.pOn.Dup(),
				pUsing: pOldItem.pUsing.Dup(),
				colUsed: pOldItem.colUsed,
			}
			if pTab := pNewItem.pTab; pTab != nil {
				pTab.nRef++
			}
		}
	}
	return
}

func (p *IdList) Dup() (pNew IdList) {
	if p != nil {
		pNew = make([]IdList, len(p))
		//	Note that because the size of the allocation for p[] is not necessarily a power of two, sqlite3IdListAppend() may not be called on the duplicate created by this function.
		for i, oldItem := range p {
			pNew[i] := &IdList_item{ Name: oldItem.Name, idx: oldItem.idx }
		}
	}
	return
}

func (p *Select) Dup() (pNew *Select) {
	if p != nil {
		pPrior := p.pPrior.Dup()
		pNew := &Select{
			pElist: p.pEList.Dup(),
			pSrc: p.pSrc.Dup(),
			Where: p.Where.Dup(),
			pGroupBy: p.pGroupBy.Dup(),
			pHaving: p.pHaving.Dup(),
			pOrderBy: p.pOrderBy.Dup(),
			op: p.op,
			pPrior: pPrior,
			pLimit: p.pLimit.Dup(),
			pOffset: p.pOffset.Dup(),
			selFlags: p.selFlags & ~SF_UsesEphemeral,
			addrOpenEphm: []int{ -1, -1, -1 },
		}
		if pPrior != nil {
			pPrior.Next = pNew
		}
	}
	return
}

//	Set the ExprList.Items[].Name element of the most recently added item on the expression list.
//	pList might be NULL following an OOM error. But pName should never be NULL.
func (pList *ExprList) SetName(Name Token, dequote bool) {
	if pList != nil {
		assert( pList.Len() > 0 )
		if pItem := &pList.Last(); dequote {
			pItem.Name = Dequote(Name)
		} else {
			pItem.Name = Name
		}
	}
}

/*
** Set the ExprList.a[].zSpan element of the most recently added item
** on the expression list.
**
** pList might be NULL following an OOM error.  But pSpan should never be
** NULL.  If a memory allocation fails, the pParse.db.mallocFailed flag
** is set.
*/
 void sqlite3ExprListSetSpan(
  Parse *pParse,          /* Parsing context */
  ExprList *pList,        /* List to which to add the span. */
  ExprSpan *pSpan         /* The span to be added */
){
  sqlite3 *db = pParse.db;
  assert( pList!=0 || db.mallocFailed );
  if( pList ){
    struct ExprList_item *pItem = &pList.Last()
    assert( pList.Len() > 0 );
    assert( db.mallocFailed || pItem.Expr==pSpan.Expr );
    pItem.zSpan = sqlite3DbStrNDup(db, (char*)pSpan.zStart, (int)(pSpan.zEnd - pSpan.zStart));
  }
}

//	Delete an entire expression list.
func (db *sqlite3) ExprListDelete(pList *ExprList) {
	int i
	struct ExprList_item *pItem
	if pList != nil {
	    assert( pList.Items != nil || len(pList.Items) == 0 )
		for _, item := range pList.Items {
			db.ExprDelete(item.Expr)
			item.Name = ""
			item.zSpan = ""
		}
		pList.Items = nil
		*pList = nil
	}
}

/*
** These routines are Walker callbacks.  Walker.u.pi is a pointer
** to an integer.  These routines are checking an expression to see
** if it is a constant.  Set *Walker.u.pi to 0 if the expression is
** not constant.
**
** These callback routines are used to implement the following:
**
**     sqlite3ExprIsConstant()
**     sqlite3ExprIsConstantNotJoin()
**     sqlite3ExprIsConstantOrFunction()
**
*/
static int exprNodeIsConstant(Walker *pWalker, Expr *pExpr){

  /* If pWalker.u.i is 3 then any term of the expression that comes from
  ** the ON or USING clauses of a join disqualifies the expression
  ** from being considered constant. */
  if( pWalker.u.i==3 && pExpr.HasAnyProperty(EP_FromJoin) ){
    pWalker.u.i = 0;
    return WRC_Abort;
  }

  switch( pExpr.op ){
    /* Consider functions to be constant if all their arguments are constant
    ** and pWalker.u.i==2 */
    case TK_FUNCTION:
      if( pWalker.u.i==2 ) return 0;
      /* Fall through */
    case TK_ID:
    case TK_COLUMN:
    case TK_AGG_FUNCTION:
    case TK_AGG_COLUMN:
      pWalker.u.i = 0;
      return WRC_Abort;
    default:
      return WRC_Continue;
  }
}
static int selectNodeIsConstant(Walker *pWalker, Select *NotUsed){
  pWalker.u.i = 0;
  return WRC_Abort;
}
static int exprIsConst(Expr *p, int initFlag){
  Walker w;
  w.u.i = initFlag;
  w.xExprCallback = exprNodeIsConstant;
  w.xSelectCallback = selectNodeIsConstant;
  w.Expr(p)
  return w.u.i;
}

/*
** Walk an expression tree.  Return 1 if the expression is constant
** and 0 if it involves variables or function calls.
**
** For the purposes of this function, a double-quoted string (ex: "abc")
** is considered a variable but a single-quoted string (ex: 'abc') is
** a constant.
*/
 int sqlite3ExprIsConstant(Expr *p){
  return exprIsConst(p, 1);
}

/*
** Walk an expression tree.  Return 1 if the expression is constant
** that does no originate from the ON or USING clauses of a join.
** Return 0 if it involves variables or function calls or terms from
** an ON or USING clause.
*/
 int sqlite3ExprIsConstantNotJoin(Expr *p){
  return exprIsConst(p, 3);
}

/*
** Walk an expression tree.  Return 1 if the expression is constant
** or a function call with constant arguments.  Return and 0 if there
** are any variables.
**
** For the purposes of this function, a double-quoted string (ex: "abc")
** is considered a variable but a single-quoted string (ex: 'abc') is
** a constant.
*/
 int sqlite3ExprIsConstantOrFunction(Expr *p){
  return exprIsConst(p, 2);
}

/*
** If the expression p codes a constant integer that is small enough
** to fit in a 32-bit integer, return 1 and put the value of the integer
** in *pValue.  If the expression is not an integer or if it is too big
** to fit in a signed 32-bit integer, return 0 and leave *pValue unchanged.
*/
 int sqlite3ExprIsInteger(Expr *p, int *pValue){
  int rc = 0;

  /* If an expression is an integer literal that fits in a signed 32-bit
  ** integer, then the EP_IntValue flag will have already been set */
  assert( p.op!=TK_INTEGER || (p.flags & EP_IntValue)!=0
           || sqlite3GetInt32(p.Token, &rc)==0 );

  if( p.flags & EP_IntValue ){
    *pValue = p.Value;
    return 1;
  }
  switch( p.op ){
    case TK_UPLUS: {
      rc = sqlite3ExprIsInteger(p.pLeft, pValue);
      break;
    }
    case TK_UMINUS: {
      int v;
      if( sqlite3ExprIsInteger(p.pLeft, &v) ){
        *pValue = -v;
        rc = 1;
      }
      break;
    }
    default: break;
  }
  return rc;
}

/*
** Return FALSE if there is no chance that the expression can be NULL.
**
** If the expression might be NULL or if the expression is too complex
** to tell return TRUE.  
**
** This routine is used as an optimization, to skip OP_IsNull opcodes
** when we know that a value cannot be NULL.  Hence, a false positive
** (returning TRUE when in fact the expression can never be NULL) might
** be a small performance hit but is otherwise harmless.  On the other
** hand, a false negative (returning FALSE when the result could be NULL)
** will likely result in an incorrect answer.  So when in doubt, return
** TRUE.
*/
 int sqlite3ExprCanBeNull(const Expr *p){
  byte op;
  while( p.op==TK_UPLUS || p.op==TK_UMINUS ){ p = p.pLeft; }
  op = p.op;
  if( op==TK_REGISTER ) op = p.op2;
  switch( op ){
    case TK_INTEGER:
    case TK_STRING:
    case TK_FLOAT:
    case TK_BLOB:
      return 0;
    default:
      return 1;
  }
}

/*
** Generate an OP_IsNull instruction that tests register iReg and jumps
** to location iDest if the value in iReg is NULL.  The value in iReg 
** was computed by pExpr.  If we can look at pExpr at compile-time and
** determine that it can never generate a NULL, then the OP_IsNull operation
** can be omitted.
*/
 void sqlite3ExprCodeIsNullJump(
  Vdbe *v,            /* The VDBE under construction */
  const Expr *pExpr,  /* Only generate OP_IsNull if this expr can be NULL */
  int iReg,           /* Test the value in this register for NULL */
  int iDest           /* Jump here if the value is null */
){
  if( sqlite3ExprCanBeNull(pExpr) ){
    v.AddOp2(OP_IsNull, iReg, iDest);
  }
}

/*
** Return TRUE if the given expression is a constant which would be
** unchanged by OP_Affinity with the affinity given in the second
** argument.
**
** This routine is used to determine if the OP_Affinity operation
** can be omitted.  When in doubt return FALSE.  A false negative
** is harmless.  A false positive, however, can result in the wrong
** answer.
*/
 int sqlite3ExprNeedsNoAffinityChange(const Expr *p, char aff){
  byte op;
  if( aff==SQLITE_AFF_NONE ) return 1;
  while( p.op==TK_UPLUS || p.op==TK_UMINUS ){ p = p.pLeft; }
  op = p.op;
  if( op==TK_REGISTER ) op = p.op2;
  switch( op ){
    case TK_INTEGER: {
      return aff==SQLITE_AFF_INTEGER || aff==SQLITE_AFF_NUMERIC;
    }
    case TK_FLOAT: {
      return aff==SQLITE_AFF_REAL || aff==SQLITE_AFF_NUMERIC;
    }
    case TK_STRING: {
      return aff==SQLITE_AFF_TEXT;
    }
    case TK_BLOB: {
      return 1;
    }
    case TK_COLUMN: {
      assert( p.iTable>=0 );  /* p cannot be part of a CHECK constraint */
      return p.iColumn<0
          && (aff==SQLITE_AFF_INTEGER || aff==SQLITE_AFF_NUMERIC);
    }
    default: {
      return 0;
    }
  }
}

/*
** Return TRUE if the given string is a row-id column name.
*/
 int sqlite3IsRowid(const char *z){
  if CaseInsensitiveMatch(z, "_ROWID_") || CaseInsensitiveMatch(z, "ROWID") || CaseInsensitiveMatch(z, "OID")  {
	  return 1
	}
  return 0;
}

/*
** Return true if we are able to the IN operator optimization on a
** query of the form
**
**       x IN (SELECT ...)
**
** Where the SELECT... clause is as specified by the parameter to this
** routine.
**
** The Select object passed in has already been preprocessed and no
** errors have been found.
*/
static int isCandidateForInOpt(Select *p){
  SrcList *pSrc;
  ExprList *pEList;
  Table *pTab;
  if( p==0 ) return 0;                   /* right-hand side of IN is SELECT */
  if( p.pPrior ) return 0;              /* Not a compound SELECT */
  if( p.selFlags & (SF_Distinct|SF_Aggregate) ){
    return 0; /* No DISTINCT keyword and no aggregate functions */
  }
  assert( p.pGroupBy==0 );              /* Has no GROUP BY clause */
  if( p.pLimit ) return 0;              /* Has no LIMIT clause */
  assert( p.pOffset==0 );               /* No LIMIT means no OFFSET */
  if( p.Where ) return 0;              /* Has no WHERE clause */
  pSrc = p.pSrc;
  assert( pSrc!=0 );
  if( pSrc.nSrc!=1 ) return 0;          /* Single term in FROM clause */
  if( pSrc.a[0].Select ) return 0;     /* FROM is not a subquery or view */
  pTab = pSrc.a[0].pTab;
  if( pTab==0 ) return 0;
  assert( pTab.Select==0 );            /* FROM clause is not a view */
  if( pTab.IsVirtual() ) return 0;        /* FROM clause not a virtual table */
  pEList = p.pEList;
  if( pEList.nExpr!=1 ) return 0;       /* One column in the result set */
  if( pEList.a[0].Expr.op!=TK_COLUMN ) return 0; /* Result is a column */
  return 1;
}

//	Code an OP_Once instruction and allocate space for its flag. Return the address of the new instruction.
func (pParse *Parse) CodeOnce() (address int) {
	address = pParse.GetVdbe().AddOp1(OP_Once, pParse.nOnce)
	pParse.nOnce++
	return
}

//	This function is used by the implementation of the IN (...) operator. It's job is to find or create a b-tree structure that may be used either to test for membership of the (...) set or to iterate through its members, skipping duplicates.
//	The index of the cursor opened on the b-tree (database table, database index or ephermal table) is stored in pX.iTable before this function returns. The returned value of this function indicates the b-tree type, as follows:
//			IN_INDEX_ROWID - The cursor was opened on a database table.
//			IN_INDEX_INDEX - The cursor was opened on a database index.
//			IN_INDEX_EPH -   The cursor was opened on a specially created and populated epheremal table.
//	An existing b-tree may only be used if the SELECT is of the simple form:
//			SELECT <column> FROM <table>
//	If the prNotFound parameter is 0, then the b-tree will be used to iterate through the set members, skipping any duplicates. In this case an epheremal table must be used unless the selected <column> is guaranteed to be unique - either because it is an INTEGER PRIMARY KEY or it has a UNIQUE constraint or UNIQUE index.
//	If the prNotFound parameter is not 0, then the b-tree will be used for fast set membership tests. In this case an epheremal table must be used unless <column> is an INTEGER PRIMARY KEY or an index can be found with <column> as its left-most column.
//	When the b-tree is being used for membership tests, the calling function needs to know whether or not the structure contains an SQL NULL value in order to correctly evaluate expressions like "X IN (Y, Z)". If there is any chance that the (...) might contain a NULL value at runtime, then a register is allocated and the register number written to *prNotFound. If there is no chance that the (...) contains a NULL value, then *prNotFound is left unchanged.
//	If a register is allocated and its location stored in *prNotFound, then its initial value is NULL.  If the (...) does not remain constant for the duration of the query (i.e. the SELECT within the (...) is a correlated subquery) then the value of the allocated register is reset to NULL each time the subquery is rerun. This allows the caller to use vdbe code equivalent to the following:
//			if( register==NULL ){
//				has_null = <test if data structure contains null>
//				register = 1
//			}
//	in order to avoid running the <test if data structure contains null> test more often than is necessary.
int sqlite3FindInIndex(Parse *pParse, Expr *pX, int *prNotFound){
	var p		*Select						//	SELECT to the right of IN operator

	v := pParse.GetVdbe()
	mustBeUnique := prNotFound == nil
	iTab := len(pParse.pTab) + 1			//	Cursor of the RHS table
	eType := 0
	assert( pX.op == TK_IN )				//	Type of RHS table. IN_INDEX_*

	//	Check to see if an existing table or index can be used to satisfy the query. This is preferable to generating a new ephemeral table.
	if pX.HasProperty(EP_xIsSelect) {
		p = pX.x.Select
	}

	if pParse.nErr == 0 && isCandidateForInOpt(p) {
		db := pParse.db
		assert( p )
		assert( p.pEList != nil )
		assert( p.pEList.a[0].Expr != nil )
		assert( p.pSrc != nil )
		pTab := p.pSrc.a[0].pTab
		pExpr := p.pEList.a[0].Expr
		iCol := pExpr.iColumn
   
		//	Code an OP_VerifyCookie and OP_TableLock for <table>.
		iDb := db.SchemaToIndex(pTab.Schema)
		pParse.CodeVerifySchema(iDb)
		pParse.TableLock(iDb, pTab.tnum, pTab.Name, false)

		//	This function is only called from two places. In both cases the vdbe has already been allocated. So assume GetVdbe() is always successful here.
		assert(v)
		if iCol < 0 {
			iAddr := pParse.CodeOnce()
			pParse.OpenTable(pTab, iTab, iDb, OP_OpenRead)
			eType = IN_INDEX_ROWID
			v.JumpHere(iAddr)
		} else {
			Index *pIdx;                         /* Iterator variable */

			//	The collation sequence used by the comparison. If an index is to be used in place of a temp-table, it must be ordered according to this collation sequence.
			pReq := sqlite3BinaryCompareCollSeq(pParse, pX.pLeft, pExpr)

			//	Check that the affinity that will be used to perform the comparison is the same as the affinity of the column. If it is not, it is not possible to use any index.
			aff := comparisonAffinity(pX)
			affinity_ok := pTab.Columns[iCol].affinity == aff || aff == SQLITE_AFF_NONE

			for _, pIdx = range pTab.Indices {
				if eType != 0 || !affinity_ok {
					break
				}
				if (pIdx.Columns[0] == iCol) && sqlite3FindCollSeq(db, db.Encoding(), pIdx.azColl[0], 0) == pReq && (!mustBeUnique || (len(pIdx.Columns) == 1 && pIdx.onError != OE_None)) {
					pKey := (char *)pParse.IndexKeyinfo(pIdx)
					iAddr := pParse.CodeOnce()
					sqlite3VdbeAddOp4(v, OP_OpenRead, iTab, pIdx.tnum, iDb, pKey,P4_KEYINFO_HANDOFF)
					v.Comment(pIdx.Name)
					eType = IN_INDEX_INDEX

					v.JumpHere(iAddr)
					if prNotFound && !pTab.Columns[iCol].notNull {
						*prNotFound = ++pParse.nMem
						v.AddOp2(OP_Null, 0, *prNotFound)
					}
				}
			}
		}
	}

	if eType == 0 {
		//	Could not found an existing table or index to use as the RHS b-tree. We will have to generate an ephemeral table to do the job.
		savedNQueryLoop := pParse.nQueryLoop
		rMayHaveNull := 0
		eType = IN_INDEX_EPH
		if prNotFound {
			*prNotFound = rMayHaveNull = ++pParse.nMem
			v.AddOp2(OP_Null, 0, *prNotFound)
		} else {
			pParse.nQueryLoop = float64(1)
			if pX.pLeft.iColumn < 0 && !pX.HasAnyProperty(EP_xIsSelect) {
				eType = IN_INDEX_ROWID
			}
		}
		sqlite3CodeSubselect(pParse, pX, rMayHaveNull, eType == IN_INDEX_ROWID)
		pParse.nQueryLoop = savedNQueryLoop
	} else {
		pX.iTable = iTab
	}
	return eType
}

/*
** Generate code for scalar subqueries used as a subquery expression, EXISTS,
** or IN operators.  Examples:
**
**     (SELECT a FROM b)          -- subquery
**     EXISTS (SELECT a FROM b)   -- EXISTS subquery
**     x IN (4,5,11)              -- IN operator with list on right-hand side
**     x IN (SELECT a FROM b)     -- IN operator with subquery on the right
**
** The pExpr parameter describes the expression that contains the IN
** operator or subquery.
**
** If parameter isRowid is non-zero, then expression pExpr is guaranteed
** to be of the form "<rowid> IN (?, ?, ?)", where <rowid> is a reference
** to some integer key column of a table B-Tree. In this case, use an
** intkey B-Tree to store the set of IN(...) values instead of the usual
** (slower) variable length keys B-Tree.
**
** If rMayHaveNull is non-zero, that means that the operation is an IN
** (not a SELECT or EXISTS) and that the RHS might contains NULLs.
** Furthermore, the IN is in a WHERE clause and that we really want
** to iterate over the RHS of the IN operator in order to quickly locate
** all corresponding LHS elements.  All this routine does is initialize
** the register given by rMayHaveNull to NULL.  Calling routines will take
** care of changing this register value to non-NULL if the RHS is NULL-free.
**
** If rMayHaveNull is zero, that means that the subquery is being used
** for membership testing only.  There is no need to initialize any
** registers to indicate the presense or absence of NULLs on the RHS.
**
** For a SELECT or EXISTS operator, return the register that holds the
** result.  For IN operators or if an error occurs, the return value is 0.
*/
int sqlite3CodeSubselect(
	Parse *pParse,          /* Parsing context */
	Expr *pExpr,            /* The IN, SELECT, or EXISTS operator */
	int rMayHaveNull,       /* Register that records whether NULLs exist in RHS */
	int isRowid             /* If true, LHS of IN operator is a rowid */
){
	int testAddr = -1;                      /* One-time test address */
	int rReg = 0;                           /* Register storing resulting */
	Vdbe *v = pParse.GetVdbe()
	if v == nil {
		return 0
	}
	pParse.ColumnCacheContext(func() {
		//	This code must be run in its entirety every time it is encountered if any of the following is true:
		//		*  The right-hand side is a correlated subquery
		//		*  The right-hand side is an expression list containing variables
		//		*  We are inside a trigger
		//	If all of the above are false, then we can run this code just once save the results, and reuse the same result on subsequent invocations.
		if !pExpr.HasAnyProperty(EP_VarSelect) {
			testAddr = pParse.CodeOnce()
		}

#ifndef SQLITE_OMIT_EXPLAIN
		if pParse.explain == 2 {
			message := fmt.Sprintf("EXECUTE %v%v SUBQUERY %v", testAddr >= 0 ? "" : "CORRELATED ", pExpr.op == TK_IN ? "LIST" : "SCALAR", pParse.iNextSelectId)
			sqlite3VdbeAddOp4(v, OP_Explain, pParse.iSelectId, 0, 0, message, P4_DYNAMIC)
		}
#endif

		switch pExpr.op {
		case TK_IN:
			char affinity;              /* Affinity of the LHS of the IN */
			KeyInfo keyInfo;            /* Keyinfo for the generated table */
			pLeft := pExpr.pLeft

			if rMayHaveNull {
				v.AddOp2(OP_Null, 0, rMayHaveNull)
			}
			affinity = sqlite3ExprAffinity(pLeft)

			//	Whether this is an 'x IN(SELECT...)' or an 'x IN(<exprlist>)' expression it is handled the same way. An ephemeral table is filled with single-field index keys representing the results from the SELECT or the <exprlist>.
			//	If the 'x' expression is a column value, or the SELECT... statement returns a column value, then the affinity of that column is used to build the index keys. If both 'x' and the SELECT... statement are columns, then numeric affinity is used if either column has NUMERIC or INTEGER affinity. If neither 'x' nor the SELECT... statement are columns, then numeric affinity is used.
			pExpr.iTable = pParse.nTab++
			addr := v.AddOp2(OP_OpenEphemeral, pExpr.iTable, !isRowid)
			if rMayHaveNull == 0 {
				v.ChangeP5(BTREE_UNORDERED)
			}
			memset(&keyInfo, 0, sizeof(keyInfo))
			keyInfo.nField = 1

			switch {
			case pExpr.HasProperty(EP_xIsSelect):
				//	Case 1:     expr IN (SELECT ...)
				//	Generate code to write the results of the select into the temporary table allocated and opened above.
				SelectDest dest;
				ExprList *pEList;

				assert( !isRowid )
				sqlite3SelectDestInit(&dest, SRT_Set, pExpr.iTable)
				dest.affinity = byte(affinity)
				assert( (pExpr.iTable & 0x0000FFFF) == pExpr.iTable )
				pExpr.x.Select.iLimit = 0
				if sqlite3Select(pParse, pExpr.x.Select, &dest) {
					return 0
				}
				pEList = pExpr.x.Select.pEList
				if pEList!=0 && pEList.nExpr > 0 { 
					keyInfo.Collations[0] = sqlite3BinaryCompareCollSeq(pParse, pExpr.pLeft, pEList.a[0].Expr)
				}
			case pExpr.pList != nil:
				//	Case 2:     expr IN (exprlist)
				//	For each expression, build an index key from the evaluation and store it in the temporary table. If <expr> is a column, then use that columns affinity when building index keys. If <expr> is not a column, use numeric affinity.
				int i
				pList := pExpr.pList
				struct ExprList_item *pItem
				int r1, r2, r3
				if !affinity {
					affinity = SQLITE_AFF_NONE
				}
				keyInfo.Collations[0] = sqlite3ExprCollSeq(pParse, pExpr.pLeft)

				//	Loop through each expression in <exprlist>.
				r1 = pParse.GetTempReg()
				r2 = pParse.GetTempReg()
				v.AddOp2(OP_Null, 0, r2)
				for(i=pList.nExpr, pItem=pList.a; i>0; i--, pItem++){
					pE2 := pItem.Expr
					int iValToIns
					//	If the expression is not constant then we will need to disable the test that was generated above that makes sure this code only executes once. Because for a non-constant expression we need to rerun this code each time.
					if testAddr >= 0 && !sqlite3ExprIsConstant(pE2) {
						v.ChangeToNoop(testAddr)
						testAddr = -1
					}
					//	Evaluate the expression and insert it into the temp table
					if isRowid && sqlite3ExprIsInteger(pE2, &iValToIns) {
						v.AddOp3(OP_InsertInt, pExpr.iTable, r2, iValToIns)
					} else {
						r3 = sqlite3ExprCodeTarget(pParse, pE2, r1)
						if isRowid {
							v.AddOp2(OP_MustBeInt, r3, v.CurrentAddr() + 2)
							v.AddOp3(OP_Insert, pExpr.iTable, r2, r3)
						} else {
							sqlite3VdbeAddOp4(v, OP_MakeRecord, r3, 1, r2, &affinity, 1)
							sqlite3ExprCacheAffinityChange(pParse, r3, 1)
							v.AddOp2(OP_IdxInsert, pExpr.iTable, r2)
						}
					}
				}
				pParse.ReleaseTempReg(r1)
				pParse.ReleaseTempReg(r2)
			}
			if !isRowid {
				sqlite3VdbeChangeP4(v, addr, (void *)&keyInfo, P4_KEYINFO)
			}
		case TK_EXISTS:
		case TK_SELECT:
		default:
			//	If this has to be a scalar SELECT. Generate code to put the value of this select in a memory cell and record the number of the memory cell in iColumn. If this is an EXISTS, write an integer 0 (not exists) or 1 (exists) into a memory cell and record that memory cell in iColumn.
			Select *pSel;                         /* SELECT statement to encode */
			SelectDest dest;                      /* How to deal with SELECt result */
			assert( pExpr.op == TK_EXISTS || pExpr.op == TK_SELECT )
			assert( pExpr.HasProperty(EP_xIsSelect) )
			pSel = pExpr.x.Select
			sqlite3SelectDestInit(&dest, 0, ++pParse.nMem)
			if pExpr.op == TK_SELECT {
				dest.eDest = SRT_Mem
				v.AddOp2(OP_Null, 0, dest.iParm)
				v.Comment("Init subquery result")
			} else {
				dest.eDest = SRT_Exists
				v.AddOp2(OP_Integer, 0, dest.iParm)
				v.Comment("Init EXISTS result")
			}
			pParse.db.ExprDelete(pSel.pLimit)
			pSel.pLimit = pParse.Expr(TK_INTEGER, nil, nil, &sqlite3IntTokens[1])
			pSel.iLimit = 0
			if sqlite3Select(pParse, pSel, &dest) {
				return 0
			}
			rReg = dest.iParm
		}

		if testAddr >= 0 {
			v.JumpHere(testAddr)
		}
	})
	return rReg
}

//	Generate code for an IN expression.
//			x IN (SELECT ...)
//			x IN (value, value, ...)
//	The left-hand side (LHS) is a scalar expression. The right-hand side (RHS) is an array of zero or more values. The expression is true if the LHS is contained within the RHS. The value of the expression is unknown (NULL) if the LHS is NULL or if the LHS is not contained within the RHS and the RHS contains one or more NULL values.
//	This routine generates code will jump to destIfFalse if the LHS is not contained within the RHS. If due to NULLs we cannot determine if the LHS is contained in the RHS then jump to destIfNull. If the LHS is contained within the RHS then fall through.
static void sqlite3ExprCodeIN(
  Parse *pParse,        /* Parsing and code generating context */
  Expr *pExpr,          /* The IN expression */
  int destIfFalse,      /* Jump here if LHS is not contained in the RHS */
  int destIfNull        /* Jump here if the results are unknown due to NULLs */
){
	int rRhsHasNull = 0;  /* Register that is true if RHS contains NULL values */
	char affinity;        /* Comparison affinity to use */
	int r1;               /* Temporary use register */
	Vdbe *v;              /* Statement under construction */

	//	Compute the RHS. After this step, the table with cursor pExpr.iTable will contains the values that make up the RHS.
	v = pParse.pVdbe
	assert( v != nil )			//	OOM detected prior to this routine
	v.NoopComment("begin IN expr")
	eType := sqlite3FindInIndex(pParse, pExpr, &rRhsHasNull)

	//	Figure out the affinity to use to create a key from the results of the expression. affinityStr stores a static string suitable for P4 of OP_MakeRecord.
	affinity = comparisonAffinity(pExpr)

	//	Code the LHS, the <expr> from "<expr> IN (...)".
	pParse.ColumnCacheContext(func() {
		r1 = pParse.GetTempReg()
		sqlite3ExprCode(pParse, pExpr.pLeft, r1)

		//	If the LHS is NULL, then the result is either false or NULL depending on whether the RHS is empty or not, respectively.
		if destIfNull == destIfFalse {
			//	Shortcut for the common case where the false and NULL outcomes are the same.
			v.AddOp2(OP_IsNull, r1, destIfNull)
		} else {
			addr1 := v.AddOp1(OP_NotNull, r1)
			v.AddOp2(OP_Rewind, pExpr.iTable, destIfFalse)
			v.AddOp2(OP_Goto, 0, destIfNull)
			v.JumpHere(addr1)
		}

		if eType == IN_INDEX_ROWID {
			//	In this case, the RHS is the ROWID of table b-tree
			v.AddOp2(OP_MustBeInt, r1, destIfFalse)
			v.AddOp3(OP_NotExists, pExpr.iTable, destIfFalse, r1)
		} else {
			//	In this case, the RHS is an index b-tree.
			sqlite3VdbeAddOp4(v, OP_Affinity, r1, 1, 0, &affinity, 1)

			//	If the set membership test fails, then the result of the "x IN (...)" expression must be either 0 or NULL. If the set contains no NULL values, then the result is 0. If the set contains one or more NULL values, then the result of the expression is also NULL.
			if rRhsHasNull == 0 || destIfFalse == destIfNull {
				//	This branch runs if it is known at compile time that the RHS cannot contain NULL values. This happens as the result of a "NOT NULL" constraint in the database schema.
				//	Also run this branch if NULL is equivalent to FALSE for this particular IN operator.
				sqlite3VdbeAddOp4Int(v, OP_NotFound, pExpr.iTable, destIfFalse, r1, 1)
			} else {
				//	In this branch, the RHS of the IN might contain a NULL and the presence of a NULL on the RHS makes a difference in the outcome.
				int j1, j2, j3

				//	First check to see if the LHS is contained in the RHS. If so, then the presence of NULLs in the RHS does not matter, so jump over all of the code that follows.
				j1 = sqlite3VdbeAddOp4Int(v, OP_Found, pExpr.iTable, 0, r1, 1)

				//	Here we begin generating code that runs if the LHS is not contained within the RHS. Generate additional code that tests the RHS for NULLs. If the RHS contains a NULL then jump to destIfNull. If there are no NULLs in the RHS then jump to destIfFalse.
				j2 = v.AddOp1(OP_NotNull, rRhsHasNull)
				j3 = sqlite3VdbeAddOp4Int(v, OP_Found, pExpr.iTable, 0, rRhsHasNull, 1)
				v.AddOp2(OP_Integer, -1, rRhsHasNull)
				v.JumpHere(j3)
				v.AddOp2(OP_AddImm, rRhsHasNull, 1)
				v.JumpHere(j2)

				//	Jump to the appropriate target depending on whether or not the RHS contains a NULL
				v.AddOp2(OP_If, rRhsHasNull, destIfNull)
				v.AddOp2(OP_Goto, 0, destIfFalse)

				//	The OP_Found at the top of this branch jumps here when true, causing the overall IN expression evaluation to fall through.
				v.JumpHere(j1)
			}
		}
		pParse.ReleaseTempReg(r1)
	})
  v.Comment("end IN expr")
}

/*
** Duplicate an 8-byte value
*/
static char *dup8bytes(Vdbe *v, const char *in){
  char *out = sqlite3DbMallocRaw(v.DB(), 8);
  if( out ){
    memcpy(out, in, 8);
  }
  return out;
}

/*
** Generate an instruction that will put the floating point
** value described by z[0..n-1] into register iMem.
**
** The z[] string will probably not be zero-terminated.  But the 
** z[n] character is guaranteed to be something that does not look
** like the continuation of the number.
*/
static void codeReal(Vdbe *v, const char *z, int negateFlag, int iMem){
  if( z!=0 ){
    double value;
    char *zV;
    sqlite3AtoF(z, &value, sqlite3Strlen30(z), SQLITE_UTF8);
    assert( !math.IsNaN(value) ); /* The new AtoF never returns NaN */
    if( negateFlag ) value = -value;
    zV = dup8bytes(v, (char*)&value);
    sqlite3VdbeAddOp4(v, OP_Real, 0, iMem, 0, zV, P4_REAL);
  }
}


/*
** Generate an instruction that will put the integer describe by
** text z[0..n-1] into register iMem.
**
** Expr.Token is always UTF8 and zero-terminated.
*/
static void codeInteger(Parse *pParse, Expr *pExpr, int negFlag, int iMem){
  Vdbe *v = pParse.pVdbe;
  if( pExpr.flags & EP_IntValue ){
    int i = pExpr.Value;
    assert( i>=0 );
    if( negFlag ) i = -i;
    v.AddOp2(OP_Integer, i, iMem);
  }else{
    int c;
    int64 value;
    const char *z = pExpr.Token;
    assert( z!=0 );
    c = Atoint64(z, &value, sqlite3Strlen30(z), SQLITE_UTF8);
    if( c==0 || (c==2 && negFlag) ){
      char *zV;
      if( negFlag ){ value = c==2 ? SMALLEST_INT64 : -value; }
      zV = dup8bytes(v, (char*)&value);
      sqlite3VdbeAddOp4(v, OP_Int64, 0, iMem, 0, zV, P4_INT64);
    }else{
      codeReal(v, z, negFlag, iMem);
    }
  }
}

/*
** Clear a cache entry.
*/
static void cacheEntryClear(Parse *pParse, struct yColCache *p){
	if p.needsFreeing {
		pParse.aTempReg = append(pParse.aTempReg(p.iReg))
		p.needsFreeing = false
	}
}


/*
** Record in the column cache that a particular column from a
** particular table is stored in a particular register.
*/
 void sqlite3ExprCacheStore(Parse *pParse, int iTab, int iCol, int iReg){
  int i;
  int minLru;
  int idxLru;
  struct yColCache *p;

  assert( iReg>0 );  /* Register numbers are always positive */
  assert( iCol>=-1 && iCol<32768 );  /* Finite column numbers */

  /* Find an empty slot and replace it */
  for i, p := range pParse.ColumnsCache {
    if p.iReg == 0 {
      p.iLevel = pParse.iCacheLevel;
      p.iTable = iTab;
      p.iColumn = iCol;
      p.iReg = iReg;
      p.needsFreeing = false
      p.lru = pParse.iCacheCnt++;
      return;
    }
  }

  /* Replace the last recently used */
  minLru = 0x7fffffff;
  idxLru = -1;
  for i, p := range pParse.ColumnsCache {
    if p.lru < minLru {
      idxLru = i;
      minLru = p.lru;
    }
  }
  if( idxLru>=0 ){
    p = &pParse.ColumnsCache[idxLru];
    p.iLevel = pParse.iCacheLevel;
    p.iTable = iTab;
    p.iColumn = iCol;
    p.iReg = iReg;
    p.needsFreeing = false
    p.lru = pParse.iCacheCnt++;
    return;
  }
}

//	Indicate that registers between start_register..start_register + span - 1 are being overwritten. Purge the range of registers from the column cache.
func (pParse *Parse) ExprCacheRemove(start_register, span int) {
	iLast := start_register + span - 1
	for i, p := range pParse.ColumnsCache {
		r := p.iReg
		if r >= start_register && r <= iLast {
			cacheEntryClear(pParse, p)
			p.iReg = 0
		}
	}
}

//	Remember the current column cache context. Any new entries added added to the column cache after this call are removed when the corresponding pop occurs.
func (pParse *Parse) ExprCachePush() {
	pParse.iCacheLevel++
}

//	Remove from the column cache any entries that were added since the the previous N Push operations. In other words, restore the cache to the state it was in N Pushes ago.
func (pParse *Parse) ExprCachePop(N int) {
	assert( N > 0 )
	assert( pParse.iCacheLevel >= N )
	pParse.iCacheLevel -= N
	for i, p := range pParse.ColumnsCache {
		if p.iReg && p.iLevel > pParse.iCacheLevel {
			cacheEntryClear(pParse, p)
			p.iReg = 0
		}
	}
}

func (pParse *Parse) ColumnCacheContext(f func()) {
	pParse.ExprCachePush()
	f()
	pParse.ExprCachePop(1)
}


//	When a cached column is reused, make sure that its register is no longer available as a temp register.
//	ticket #3879: that same register might be in the cache in multiple places, so be sure to get them all.
func (pParse *Parse) ExprCachePinRegister(register int) {
	for i, p := range pParse.ColumnsCache {
		if p.iReg == register {
			p.needsFreeing = false
		}
	}
}

//	Generate code to extract the value of the iCol-th column of a table.
func (v *Vdbe) ExprCodeGetColumnOfTable(table *Table, cursor, column, register int) {
	if column < 0 || column == table.iPKey {
		v.AddOp2(OP_Rowid, cursor, register)
	} else {
		op := table.IsVirtual() ? OP_VColumn : OP_Column
		v.AddOp3(op, cursor, column, register)
	}
	if column >= 0 {
		v.ColumnDefault(table, column, register)
	}
}

//	Generate code that will extract the iColumn-th column from table pTab and store the column value in a register. An effort is made to store the column value in register iReg, but this is not guaranteed. The location of the column value is returned.
//	There must be an open cursor to pTab in iTable when this routine is called. If iColumn < 0 then code is generated that extracts the rowid.
func (pParse *Parse) ExprCodeGetColumn(table *Table, column, cursor, register int, p5 byte) int {
	v := pParse.pVdbe
	for i, p := range pParse.ColumnsCache {
		if p.iReg > 0 && p.iTable == cursor && p.iColumn == column {
			p.lru = pParse.iCacheCnt++
			pParse.ExprCachePinRegister(p.iReg)
			return p.iReg
		}
	}
	assert( v != nil )
	v.ExprCodeGetColumnOfTable(table, cursor, column, register)
	if p5 != 0 {
		v.ChangeP5(p5)
	} else {
		sqlite3ExprCacheStore(pParse, cursor, column, register)
	}
	return register
}

//	Clear all column cache entries.
func (pParse *Parse) ExprCacheClear() {
	for i, p := range pParse.ColumnsCache {
		if p.iReg != 0 {
			cacheEntryClear(pParse, p)
			p.iReg = 0
		}
	}
}

/*
** Record the fact that an affinity change has occurred on iCount
** registers starting with iStart.
*/
void sqlite3ExprCacheAffinityChange(Parse *pParse, int iStart, int iCount){
	pParse.ExprCacheRemove(iStart, iCount)
}

//	Generate code to move content from registers iFrom...iFrom + nReg - 1 over to iTo..iTo + nReg - 1. Keep the column cache up-to-date.
func (pParse *Parse) ExprCodeMove(from, to, count int) {
	if from != to {
		pParse.pVdbe.AddOp3(OP_Move, from, to, count)
		for i, p := range pParse.ColumnsCache {
			if x := p.iReg; x >= from && x < from + count {
				p.iReg += to - from
			}
		}
	}
}

/*
** Generate code to copy content from registers iFrom...iFrom+nReg-1
** over to iTo..iTo+nReg-1.
*/
 void sqlite3ExprCodeCopy(Parse *pParse, int iFrom, int iTo, int nReg){
  int i;
  if( iFrom==iTo ) return;
  for(i=0; i<nReg; i++){
    pParse.pVdbe.AddOp2(OP_Copy, iFrom+i, iTo+i);
  }
}

/*
** Generate code into the current Vdbe to evaluate the given
** expression.  Attempt to store the results in register "target".
** Return the register where results are stored.
**
** With this routine, there is no guarantee that results will
** be stored in target.  The result might be stored in some other
** register if it is convenient to do so.  The calling function
** must check the return code and move the results to the desired
** register.
*/
 int sqlite3ExprCodeTarget(Parse *pParse, Expr *pExpr, int target){
  Vdbe *v = pParse.pVdbe;  /* The VM under construction */
  int op;                   /* The opcode being coded */
  int inReg = target;       /* Results stored in register inReg */
  int regFree1 = 0;         /* If non-zero free this temporary register */
  int regFree2 = 0;         /* If non-zero free this temporary register */
  int r1, r2, r3, r4;       /* Various register numbers */
  sqlite3 *db = pParse.db; /* The database connection */

  assert( target>0 && target<=pParse.nMem );
  if( v==0 ){
    assert( pParse.db.mallocFailed );
    return 0;
  }

  if( pExpr==0 ){
    op = TK_NULL;
  }else{
    op = pExpr.op;
  }
  switch( op ){
    case TK_AGG_COLUMN: {
      AggInfo *pAggInfo = pExpr.pAggInfo;
      struct AggInfo_col *pCol = &pAggInfo.Columns[pExpr.iAgg];
      if( !pAggInfo.directMode ){
        assert( pCol.iMem>0 );
        inReg = pCol.iMem;
        break;
      }else if( pAggInfo.useSortingIdx ){
        v.AddOp3(OP_Column, pAggInfo.sortingIdxPTab, pCol.iSorterColumn, target);
        break;
      }
      /* Otherwise, fall thru into the TK_COLUMN case */
    }
    case TK_COLUMN: {
      if( pExpr.iTable<0 ){
        /* This only happens when coding check constraints */
        assert( pParse.ckBase>0 );
        inReg = pExpr.iColumn + pParse.ckBase;
      }else{
        inReg = pParse.ExprCodeGetColumn(pExpr.pTab, pExpr.iColumn, pExpr.iTable, target, pExpr.op2)
      }
      break;
    }
    case TK_INTEGER: {
      codeInteger(pParse, pExpr, 0, target);
      break;
    }
    case TK_FLOAT: {
      assert( !pExpr.HasProperty(EP_IntValue) );
      codeReal(v, pExpr.Token, 0, target);
      break;
    }
    case TK_STRING: {
      assert( !pExpr.HasProperty(EP_IntValue) );
      sqlite3VdbeAddOp4(v, OP_String8, 0, target, 0, pExpr.Token, 0);
      break;
    }
    case TK_NULL: {
      v.AddOp2(OP_Null, 0, target);
      break;
    }
    case TK_BLOB: {
      int n;
      const char *z;
      char *zBlob;
      assert( !pExpr.HasProperty(EP_IntValue) );
      assert( pExpr.Token[0]=='x' || pExpr.Token[0]=='X' );
      assert( pExpr.Token[1]=='\'' );
      z = &pExpr.Token[2];
      n = sqlite3Strlen30(z) - 1;
      assert( z[n]=='\'' );
      zBlob = sqlite3HexToBlob(v.DB(), z, n);
      sqlite3VdbeAddOp4(v, OP_Blob, n/2, target, 0, zBlob, P4_DYNAMIC);
      break;
    }
    case TK_VARIABLE: {
      assert( !pExpr.HasProperty(EP_IntValue) );
      assert( pExpr.Token!=0 );
      assert( pExpr.Token[0]!=0 );
      v.AddOp2(OP_Variable, pExpr.iColumn, target);
      if( pExpr.Token[1]!=0 ){
        assert( pExpr.Token[0]=='?' 
             || strcmp(pExpr.Token, pParse.azVar[pExpr.iColumn-1])==0 );
        sqlite3VdbeChangeP4(v, -1, pParse.azVar[pExpr.iColumn-1], P4_STATIC);
      }
      break;
    }
    case TK_REGISTER: {
      inReg = pExpr.iTable;
      break;
    }
    case TK_AS: {
      inReg = sqlite3ExprCodeTarget(pParse, pExpr.pLeft, target);
      break;
    }
#ifndef SQLITE_OMIT_CAST
    case TK_CAST: {
      /* Expressions of the form:   CAST(pLeft AS token) */
      int aff, to_op;
      inReg = sqlite3ExprCodeTarget(pParse, pExpr.pLeft, target);
      assert( !pExpr.HasProperty(EP_IntValue) );
      aff = sqlite3AffinityType(pExpr.Token);
      to_op = aff - SQLITE_AFF_TEXT + OP_ToText;
      assert( to_op==OP_ToText    || aff!=SQLITE_AFF_TEXT    );
      assert( to_op==OP_ToBlob    || aff!=SQLITE_AFF_NONE    );
      assert( to_op==OP_ToNumeric || aff!=SQLITE_AFF_NUMERIC );
      assert( to_op==OP_ToInt     || aff!=SQLITE_AFF_INTEGER );
      assert( to_op==OP_ToReal    || aff!=SQLITE_AFF_REAL    );
      if( inReg!=target ){
        v.AddOp2(OP_SCopy, inReg, target);
        inReg = target;
      }
      v.AddOp1(to_op, inReg);
      sqlite3ExprCacheAffinityChange(pParse, inReg, 1);
      break;
    }
#endif /* SQLITE_OMIT_CAST */
    case TK_LT:
    case TK_LE:
    case TK_GT:
    case TK_GE:
    case TK_NE:
    case TK_EQ: {
      assert( TK_LT==OP_Lt );
      assert( TK_LE==OP_Le );
      assert( TK_GT==OP_Gt );
      assert( TK_GE==OP_Ge );
      assert( TK_EQ==OP_Eq );
      assert( TK_NE==OP_Ne );
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, inReg, SQLITE_STOREP2);
      break;
    }
    case TK_IS:
    case TK_ISNOT: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      op = (op==TK_IS) ? TK_EQ : TK_NE;
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, inReg, SQLITE_STOREP2 | SQLITE_NULLEQ);
      break;
    }
    case TK_AND:
    case TK_OR:
    case TK_PLUS:
    case TK_STAR:
    case TK_MINUS:
    case TK_REM:
    case TK_BITAND:
    case TK_BITOR:
    case TK_SLASH:
    case TK_LSHIFT:
    case TK_RSHIFT: 
    case TK_CONCAT: {
      assert( TK_AND==OP_And );
      assert( TK_OR==OP_Or );
      assert( TK_PLUS==OP_Add );
      assert( TK_MINUS==OP_Subtract );
      assert( TK_REM==OP_Remainder );
      assert( TK_BITAND==OP_BitAnd );
      assert( TK_BITOR==OP_BitOr );
      assert( TK_SLASH==OP_Divide );
      assert( TK_LSHIFT==OP_ShiftLeft );
      assert( TK_RSHIFT==OP_ShiftRight );
      assert( TK_CONCAT==OP_Concat );
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      v.AddOp3(op, r2, r1, target);
      break;
    }
    case TK_UMINUS: {
      Expr *pLeft = pExpr.pLeft;
      assert( pLeft );
      if( pLeft.op==TK_INTEGER ){
        codeInteger(pParse, pLeft, 1, target);
      }else if( pLeft.op==TK_FLOAT ){
        assert( !pExpr.HasProperty(EP_IntValue) );
        codeReal(v, pLeft.Token, 1, target);
      }else{
        regFree1 = r1 = pParse.GetTempReg()
        v.AddOp2(OP_Integer, 0, r1);
        r2 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree2);
        v.AddOp3(OP_Subtract, r2, r1, target);
      }
      inReg = target;
      break;
    }
    case TK_BITNOT:
    case TK_NOT: {
      assert( TK_BITNOT==OP_BitNot );
      assert( TK_NOT==OP_Not );
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      inReg = target;
      v.AddOp2(op, r1, inReg);
      break;
    }
    case TK_ISNULL:
    case TK_NOTNULL: {
      int addr;
      assert( TK_ISNULL==OP_IsNull );
      assert( TK_NOTNULL==OP_NotNull );
      v.AddOp2(OP_Integer, 1, target);
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      addr = v.AddOp1(op, r1);
      v.AddOp2(OP_AddImm, target, -1);
      v.JumpHere(addr)
      break;
    }
    case TK_AGG_FUNCTION: {
      AggInfo *pInfo = pExpr.pAggInfo;
      if( pInfo==0 ){
        assert( !pExpr.HasProperty(EP_IntValue) );
        pParse.SetErrorMsg("misuse of aggregate: %v()", pExpr.Token);
      }else{
        inReg = pInfo.aFunc[pExpr.iAgg].iMem;
      }
      break;
    }
    case TK_CONST_FUNC:
    case TK_FUNCTION: {
      ExprList *pFarg;       /* List of function arguments */
      int nFarg;             /* Number of function arguments */
      FuncDef *pDef;         /* The function definition object */
      int nId;               /* Length of the function name in bytes */
      const char *zId;       /* The function name */
      int constMask = 0;     /* Mask of function arguments that are constant */
      int i;                 /* Loop counter */
      enc := db.Encoding()      /* The text encoding used by this database */
      CollSeq *pColl = 0;    /* A collating sequence */

      assert( !pExpr.HasProperty(EP_xIsSelect) );
      pFarg = pExpr.pList;
      nFarg = pFarg ? pFarg.nExpr : 0;
      assert( !pExpr.HasProperty(EP_IntValue) );
      zId = pExpr.Token;
      nId = sqlite3Strlen30(zId);
      pDef = db.FindFunction(zId, nFarg, enc, false)
      if( pDef==0 ){
        pParse.SetErrorMsg("unknown function: %v%v()", nId, zId);
        break;
      }

      /* Attempt a direct implementation of the built-in COALESCE() and
      ** IFNULL() functions.  This avoids unnecessary evalation of
      ** arguments past the first non-NULL argument.
      */
      if( pDef.flags & SQLITE_FUNC_COALESCE ){
        int endCoalesce = v.MakeLabel()
        assert( nFarg>=2 );
        sqlite3ExprCode(pParse, pFarg.a[0].Expr, target);
        for(i=1; i<nFarg; i++){
          v.AddOp2(OP_NotNull, target, endCoalesce);
          pParse.ExprCacheRemove(target, 1)
          pParse.ColumnCacheContext(func() {
			  sqlite3ExprCode(pParse, pFarg.a[i].Expr, target)
          })
        }
        v.ResolveLabel(endCoalesce)
        break;
      }


      if( pFarg ){
        r1 = pParse.GetTempRange(nFarg)

        /* For length() and typeof() functions with a column argument,
        ** set the P5 parameter to the OP_Column opcode to OPFLAG_LENGTHARG
        ** or OPFLAG_TYPEOFARG respectively, to avoid unnecessary data
        ** loading.
        */
        if( (pDef.flags & (SQLITE_FUNC_LENGTH|SQLITE_FUNC_TYPEOF))!=0 ){
          byte exprOp;
          assert( nFarg==1 );
          assert( pFarg.a[0].Expr!=0 );
          exprOp = pFarg.a[0].Expr.op;
          if( exprOp==TK_COLUMN || exprOp==TK_AGG_COLUMN ){
            assert( SQLITE_FUNC_LENGTH==OPFLAG_LENGTHARG );
            assert( SQLITE_FUNC_TYPEOF==OPFLAG_TYPEOFARG );
            pFarg.a[0].Expr.op2 = pDef.flags;
          }
        }

		pParse.ColumnCacheContext(func() {						//	Ticket 2ea2425d34be
			(pParse.ExprCodeExprList(pFarg, r1, true)
		})													//	Ticket 2ea2425d34be
      }else{
        r1 = 0;
      }
      /* Possibly overload the function if the first argument is
      ** a virtual table column.
      **
      ** For infix functions (LIKE, GLOB, REGEXP, and MATCH) use the
      ** second argument, not the first, as the argument to test to
      ** see if it is a column in a virtual table.  This is done because
      ** the left operand of infix functions (the operand we want to
      ** control overloading) ends up as the second argument to the
      ** function.  The expression "A glob B" is equivalent to 
      ** "glob(B,A).  We want to use the A in "A glob B" to test
      ** for function overloading.  But we use the B term in "glob(B,A)".
      */
      if( nFarg>=2 && (pExpr.flags & EP_InfixFunc) ){
        pDef = sqlite3VtabOverloadFunction(db, pDef, nFarg, pFarg.a[1].Expr);
      }else if( nFarg>0 ){
        pDef = sqlite3VtabOverloadFunction(db, pDef, nFarg, pFarg.a[0].Expr);
      }
      for(i=0; i<nFarg; i++){
        if( i<32 && sqlite3ExprIsConstant(pFarg.a[i].Expr) ){
          constMask |= (1<<i);
        }
        if( (pDef.flags & SQLITE_FUNC_NEEDCOLL)!=0 && !pColl ){
          pColl = sqlite3ExprCollSeq(pParse, pFarg.a[i].Expr);
        }
      }
      if( pDef.flags & SQLITE_FUNC_NEEDCOLL ){
        if( !pColl ) pColl = db.pDfltColl; 
        sqlite3VdbeAddOp4(v, OP_CollSeq, 0, 0, 0, (char *)pColl, P4_COLLSEQ);
      }
      sqlite3VdbeAddOp4(v, OP_Function, constMask, r1, target,
                        (char*)pDef, P4_FUNCDEF);
      v.ChangeP5(byte(nFarg))
      if( nFarg ){
        pParse.ReleaseTempRange(r1, nFarg)
      }
      break;
    }
    case TK_EXISTS:
    case TK_SELECT: {
      inReg = sqlite3CodeSubselect(pParse, pExpr, 0, 0);
      break;
    }
    case TK_IN: {
      int destIfFalse = v.MakeLabel()
      int destIfNull = v.MakeLabel()
      v.AddOp2(OP_Null, 0, target);
      sqlite3ExprCodeIN(pParse, pExpr, destIfFalse, destIfNull);
      v.AddOp2(OP_Integer, 1, target);
      v.ResolveLabel(destIfFalse)
      v.AddOp2(OP_AddImm, target, 0);
      v.ResolveLabel(destIfNull)
      break;
    }


    /*
    **    x BETWEEN y AND z
    **
    ** This is equivalent to
    **
    **    x>=y AND x<=z
    **
    ** X is stored in pExpr.pLeft.
    ** Y is stored in pExpr.pList.a[0].Expr.
    ** Z is stored in pExpr.pList.a[1].Expr.
    */
    case TK_BETWEEN: {
      Expr *pLeft = pExpr.pLeft;
      struct ExprList_item *pLItem = pExpr.pList.a;
      Expr *pRight = pLItem.Expr;

      r1 = sqlite3ExprCodeTemp(pParse, pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pRight, &regFree2);
      r3 = pParse.GetTempReg()
      r4 = pParse.GetTempReg()
      codeCompare(pParse, pLeft, pRight, OP_Ge,
                  r1, r2, r3, SQLITE_STOREP2);
      pLItem++;
      pRight = pLItem.Expr;
      pParse.ReleaseTempReg(regFree2)
      r2 = sqlite3ExprCodeTemp(pParse, pRight, &regFree2);
      codeCompare(pParse, pLeft, pRight, OP_Le, r1, r2, r4, SQLITE_STOREP2);
      v.AddOp3(OP_And, r3, r4, target);
      pParse.ReleaseTempReg(r3)
      pParse.ReleaseTempReg(r4)
      break;
    }
    case TK_UPLUS: {
      inReg = sqlite3ExprCodeTarget(pParse, pExpr.pLeft, target);
      break;
    }

    case TK_TRIGGER: {
      /* If the opcode is TK_TRIGGER, then the expression is a reference
      ** to a column in the new.* or old.* pseudo-tables available to
      ** trigger programs. In this case Expr.iTable is set to 1 for the
      ** new.* pseudo-table, or 0 for the old.* pseudo-table. Expr.iColumn
      ** is set to the column of the pseudo-table to read, or to -1 to
      ** read the rowid field.
      **
      ** The expression is implemented using an OP_Param opcode. The p1
      ** parameter is set to 0 for an old.rowid reference, or to (i+1)
      ** to reference another column of the old.* pseudo-table, where 
      ** i is the index of the column. For a new.rowid reference, p1 is
      ** set to (n+1), where n is the number of columns in each pseudo-table.
      ** For a reference to any other column in the new.* pseudo-table, p1
      ** is set to (n+2+i), where n and i are as defined previously. For
      ** example, if the table on which triggers are being fired is
      ** declared as:
      **
      **   CREATE TABLE t1(a, b);
      **
      ** Then p1 is interpreted as follows:
      **
      **   p1==0   .    old.rowid     p1==3   .    new.rowid
      **   p1==1   .    old.a         p1==4   .    new.a
      **   p1==2   .    old.b         p1==5   .    new.b       
      */
      Table *pTab = pExpr.pTab;
      int p1 = pExpr.iTable * (pTab.nCol+1) + 1 + pExpr.iColumn;

      assert( pExpr.iTable==0 || pExpr.iTable==1 );
      assert( pExpr.iColumn>=-1 && pExpr.iColumn<pTab.nCol );
      assert( pTab.iPKey<0 || pExpr.iColumn!=pTab.iPKey );
      assert( p1>=0 && p1<(pTab.nCol*2+2) );

      v.AddOp2(OP_Param, p1, target);
      v.Comment("%s.%s . $%d",
        (pExpr.iTable ? "new" : "old"),
        (pExpr.iColumn < 0 ? "rowid" : pExpr.pTab.Columns[pExpr.iColumn].Name),
        target
      )

      /* If the column has REAL affinity, it may currently be stored as an
      ** integer. Use OP_RealAffinity to make sure it is really real.  */
      if( pExpr.iColumn>=0 
       && pTab.Columns[pExpr.iColumn].affinity==SQLITE_AFF_REAL
      ){
        v.AddOp1(OP_RealAffinity, target);
      }
      break;
    }


    /*
    ** Form A:
    **   CASE x WHEN e1 THEN r1 WHEN e2 THEN r2 ... WHEN eN THEN rN ELSE y END
    **
    ** Form B:
    **   CASE WHEN e1 THEN r1 WHEN e2 THEN r2 ... WHEN eN THEN rN ELSE y END
    **
    ** Form A is can be transformed into the equivalent form B as follows:
    **   CASE WHEN x=e1 THEN r1 WHEN x=e2 THEN r2 ...
    **        WHEN x=eN THEN rN ELSE y END
    **
    ** X (if it exists) is in pExpr.pLeft.
    ** Y is in pExpr.pRight.  The Y is also optional.  If there is no
    ** ELSE clause and no other term matches, then the result of the
    ** exprssion is NULL.
    ** Ei is in pExpr.pList.a[i*2] and Ri is pExpr.pList.a[i*2+1].
    **
    ** The result of the expression is the Ri for the first matching Ei,
    ** or if there is no matching Ei, the ELSE term Y, or if there is
    ** no ELSE term, NULL.
    */
    default: assert( op==TK_CASE ); {
      int endLabel;                     /* GOTO label for end of CASE stmt */
      int nextCase;                     /* GOTO label for next WHEN clause */
      int nExpr;                        /* 2x number of WHEN terms */
      int i;                            /* Loop counter */
      ExprList *pEList;                 /* List of WHEN terms */
      struct ExprList_item *aListelem;  /* Array of WHEN terms */
      Expr opCompare;                   /* The X==Ei expression */
      Expr cacheX;                      /* Cached expression X */
      Expr *pX;                         /* The X expression */
      Expr *pTest = 0;                  /* X==Ei (form A) or just Ei (form B) */
      VVA_ONLY( int iCacheLevel = pParse.iCacheLevel; )

      assert( !pExpr.HasProperty(EP_xIsSelect) && pExpr.pList );
      assert((pExpr.pList.nExpr % 2) == 0);
      assert(pExpr.pList.nExpr > 0);
      pEList = pExpr.pList;
      aListelem = pEList.a;
      nExpr = pEList.nExpr;
      endLabel = v.MakeLabel()
      if( (pX = pExpr.pLeft)!=0 ){
        cacheX = *pX;
        cacheX.iTable = sqlite3ExprCodeTemp(pParse, pX, &regFree1);
        cacheX.op = TK_REGISTER;
        opCompare.op = TK_EQ;
        opCompare.pLeft = &cacheX;
        pTest = &opCompare;
        /* Ticket b351d95f9cd5ef17e9d9dbae18f5ca8611190001:
        ** The value in regFree1 might get SCopy-ed into the file result.
        ** So make sure that the regFree1 register is not reused for other
        ** purposes and possibly overwritten.  */
        regFree1 = 0;
      }
      for(i=0; i<nExpr; i += 2){
        pParse.ColumnCacheContext(func() {
			if pX != nil {
				assert( pTest != nil )
				opCompare.pRight = aListelem[i].Expr
			} else {
				pTest = aListelem[i].Expr
			}
			nextCase = v.MakeLabel()
			sqlite3ExprIfFalse(pParse, pTest, nextCase, SQLITE_JUMPIFNULL)
			sqlite3ExprCode(pParse, aListelem[i + 1].Expr, target)
			v.AddOp2(OP_Goto, 0, endLabel)
		})
        v.ResolveLabel(nextCase)
      }
      if( pExpr.pRight ){
        pParse.ColumnCacheContext(func() {
			sqlite3ExprCode(pParse, pExpr.pRight, target)
        })
      }else{
        v.AddOp2(OP_Null, 0, target);
      }
      assert( db.mallocFailed || pParse.nErr>0 || pParse.iCacheLevel==iCacheLevel );
      v.ResolveLabel(endLabel)
      break;
    }
    case TK_RAISE: {
      assert( pExpr.affinity==OE_Rollback 
           || pExpr.affinity==OE_Abort
           || pExpr.affinity==OE_Fail
           || pExpr.affinity==OE_Ignore
      );
      if( !pParse.pTriggerTab ){
        pParse.SetErrorMsg("RAISE() may only be used within a trigger-program");
        return 0;
      }
      if( pExpr.affinity==OE_Abort ){
        sqlite3MayAbort(pParse);
      }
      assert( !pExpr.HasProperty(EP_IntValue) );
      if( pExpr.affinity==OE_Ignore ){
        sqlite3VdbeAddOp4(
            v, OP_Halt, SQLITE_OK, OE_Ignore, 0, pExpr.Token,0);
      }else{
        sqlite3HaltConstraint(pParse, pExpr.affinity, pExpr.Token, 0);
      }

      break;
    }
  }
  pParse.ReleaseTempReg(regFree1)
  pParse.ReleaseTempReg(regFree2)
  return inReg;
}

/*
** Generate code to evaluate an expression and store the results
** into a register.  Return the register number where the results
** are stored.
**
** If the register is a temporary register that can be deallocated,
** then write its number into *pReg.  If the result register is not
** a temporary, then set *pReg to zero.
*/
 int sqlite3ExprCodeTemp(Parse *pParse, Expr *pExpr, int *pReg){
  int r1 = pParse.GetTempReg()
  int r2 = sqlite3ExprCodeTarget(pParse, pExpr, r1);
  if( r2==r1 ){
    *pReg = r1;
  }else{
    pParse.ReleaseTempReg(r1)
    *pReg = 0;
  }
  return r2;
}

/*
** Generate code that will evaluate expression pExpr and store the
** results in register target.  The results are guaranteed to appear
** in register target.
*/
 int sqlite3ExprCode(Parse *pParse, Expr *pExpr, int target){
  int inReg;

  assert( target>0 && target<=pParse.nMem );
  if( pExpr && pExpr.op==TK_REGISTER ){
    pParse.pVdbe.AddOp2(OP_Copy, pExpr.iTable, target);
  }else{
    inReg = sqlite3ExprCodeTarget(pParse, pExpr, target);
    assert( pParse.pVdbe || pParse.db.mallocFailed );
    if( inReg!=target && pParse.pVdbe ){
      pParse.pVdbe.AddOp2(OP_SCopy, inReg, target);
    }
  }
  return target;
}

/*
** Generate code that evalutes the given expression and puts the result
** in register target.
**
** Also make a copy of the expression results into another "cache" register
** and modify the expression so that the next time it is evaluated,
** the result is a copy of the cache register.
**
** This routine is used for expressions that are used multiple 
** times.  They are evaluated once and the results of the expression
** are reused.
*/
 int sqlite3ExprCodeAndCache(Parse *pParse, Expr *pExpr, int target){
  Vdbe *v = pParse.pVdbe;
  int inReg;
  inReg = sqlite3ExprCode(pParse, pExpr, target);
  assert( target>0 );
  /* This routine is called for terms to INSERT or UPDATE.  And the only
  ** other place where expressions can be converted into TK_REGISTER is
  ** in WHERE clause processing.  So as currently implemented, there is
  ** no way for a TK_REGISTER to exist here. */
  if( pExpr.op!=TK_REGISTER ){  
    int iMem;
    iMem = ++pParse.nMem;
    v.AddOp2(OP_Copy, inReg, iMem);
    pExpr.iTable = iMem;
    pExpr.op2 = pExpr.op;
    pExpr.op = TK_REGISTER;
  }
  return inReg;
}

/*
** Return TRUE if pExpr is an constant expression that is appropriate
** for factoring out of a loop.  Appropriate expressions are:
**
**    *  Any expression that evaluates to two or more opcodes.
**
**    *  Any OP_Integer, OP_Real, OP_String, OP_Blob, OP_Null, 
**       or OP_Variable that does not need to be placed in a 
**       specific register.
**
** There is no point in factoring out single-instruction constant
** expressions that need to be placed in a particular register.  
** We could factor them out, but then we would end up adding an
** OP_SCopy instruction to move the value into the correct register
** later.  We might as well just use the original instruction and
** avoid the OP_SCopy.
*/
static int isAppropriateForFactoring(Expr *p){
  if( !sqlite3ExprIsConstantNotJoin(p) ){
    return 0;  /* Only constant expressions are appropriate for factoring */
  }
  if( (p.flags & EP_FixedDest)==0 ){
    return 1;  /* Any constant without a fixed destination is appropriate */
  }
  while( p.op==TK_UPLUS ) p = p.pLeft;
  switch( p.op ){
    case TK_BLOB:
    case TK_VARIABLE:
    case TK_INTEGER:
    case TK_FLOAT:
    case TK_NULL:
    case TK_STRING: {
      /* Single-instruction constants with a fixed destination are
      ** better done in-line.  If we factor them, they will just end
      ** up generating an OP_SCopy to move the value to the destination
      ** register. */
      return 0;
    }
    case TK_UMINUS: {
      if( p.pLeft.op==TK_FLOAT || p.pLeft.op==TK_INTEGER ){
        return 0;
      }
      break;
    }
    default: {
      break;
    }
  }
  return 1;
}

/*
** If pExpr is a constant expression that is appropriate for
** factoring out of a loop, then evaluate the expression
** into a register and convert the expression into a TK_REGISTER
** expression.
*/
static int evalConstExpr(Walker *pWalker, Expr *pExpr){
  Parse *pParse = pWalker.pParse;
  switch( pExpr.op ){
    case TK_IN:
    case TK_REGISTER: {
      return WRC_Prune;
    }
    case TK_FUNCTION:
    case TK_AGG_FUNCTION:
    case TK_CONST_FUNC: {
      /* The arguments to a function have a fixed destination.
      ** Mark them this way to avoid generated unneeded OP_SCopy
      ** instructions. 
      */
      ExprList *pList = pExpr.pList;
      assert( !pExpr.HasProperty(EP_xIsSelect) );
      if( pList ){
        int i = pList.nExpr;
        struct ExprList_item *pItem = pList.a;
        for(; i>0; i--, pItem++){
          if( pItem.Expr ) pItem.Expr.flags |= EP_FixedDest;
        }
      }
      break;
    }
  }
  if( isAppropriateForFactoring(pExpr) ){
    int r1 = ++pParse.nMem;
    int r2;
    r2 = sqlite3ExprCodeTarget(pParse, pExpr, r1);
    if r1 != r2 {
		pParse.ReleaseTempReg(r1)
	}
    pExpr.op2 = pExpr.op;
    pExpr.op = TK_REGISTER;
    pExpr.iTable = r2;
    return WRC_Prune;
  }
  return WRC_Continue;
}

/*
** Preevaluate constant subexpressions within pExpr and store the
** results in registers.  Modify pExpr so that the constant subexpresions
** are TK_REGISTER opcodes that refer to the precomputed values.
**
** This routine is a no-op if the jump to the cookie-check code has
** already occur.  Since the cookie-check jump is generated prior to
** any other serious processing, this check ensures that there is no
** way to accidently bypass the constant initializations.
*/
 void sqlite3ExprCodeConstants(Parse *pParse, Expr *pExpr){
  Walker w;
  if( pParse.cookieGoto ) return;
  w.xExprCallback = evalConstExpr;
  w.xSelectCallback = 0;
  w.pParse = pParse;
  w.Expr(pExpr)
}


//	Generate code that pushes the value of every element of the given expression list into a sequence of registers beginning at target.
//	Return the number of elements evaluated.
func (pParse *Parse) ExprCodeExprList(pList *ExprList, target int, doHardCopy bool) (n int) {
	assert( pList != nil )
	assert( target > 0 )
	assert( pParse.pVdbe != nil )		//	Never gets this far otherwise
	n = pList.Len()
	if doHardCopy {
		for i, item := range pList.Items {
			t := target + i
			if inReg := sqlite3ExprCodeTarget(pParse, item.Expr, t); inReg != t {
				pParse.pVdbe.AddOp2(OP_Copy, inReg, t)
			}
		}
	} else {
		for i, item := range pList.Items {
			t := target + i
			if inReg := sqlite3ExprCodeTarget(pParse, item.Expr, t); inReg != t {
				pParse.pVdbe.AddOp2(OP_SCopy, inReg, t)
			}
		}
	}
	return
}

/*
** Generate code for a BETWEEN operator.
**
**    x BETWEEN y AND z
**
** The above is equivalent to 
**
**    x>=y AND x<=z
**
** Code it as such, taking care to do the common subexpression
** elementation of x.
*/
static void exprCodeBetween(
  Parse *pParse,    /* Parsing and code generating context */
  Expr *pExpr,      /* The BETWEEN expression */
  int dest,         /* Jump here if the jump is taken */
  int jumpIfTrue,   /* Take the jump if the BETWEEN is true */
  int jumpIfNull    /* Take the jump if the BETWEEN is NULL */
){
  Expr exprAnd;     /* The AND operator in  x>=y AND x<=z  */
  Expr compLeft;    /* The  x>=y  term */
  Expr compRight;   /* The  x<=z  term */
  Expr exprX;       /* The  x  subexpression */
  int regFree1 = 0; /* Temporary use register */

  assert( !pExpr.HasProperty(EP_xIsSelect) );
  exprX = *pExpr.pLeft;
  exprAnd.op = TK_AND;
  exprAnd.pLeft = &compLeft;
  exprAnd.pRight = &compRight;
  compLeft.op = TK_GE;
  compLeft.pLeft = &exprX;
  compLeft.pRight = pExpr.pList.a[0].Expr;
  compRight.op = TK_LE;
  compRight.pLeft = &exprX;
  compRight.pRight = pExpr.pList.a[1].Expr;
  exprX.iTable = sqlite3ExprCodeTemp(pParse, &exprX, &regFree1);
  exprX.op = TK_REGISTER;
  if( jumpIfTrue ){
    sqlite3ExprIfTrue(pParse, &exprAnd, dest, jumpIfNull);
  }else{
    sqlite3ExprIfFalse(pParse, &exprAnd, dest, jumpIfNull);
  }
  pParse.ReleaseTempReg(regFree1)
}

/*
** Generate code for a boolean expression such that a jump is made
** to the label "dest" if the expression is true but execution
** continues straight thru if the expression is false.
**
** If the expression evaluates to NULL (neither true nor false), then
** take the jump if the jumpIfNull flag is SQLITE_JUMPIFNULL.
**
** This code depends on the fact that certain token values (ex: TK_EQ)
** are the same as opcode values (ex: OP_Eq) that implement the corresponding
** operation.  Special comments in vdbe.c and the mkopcodeh.awk script in
** the make process cause these values to align.  Assert()s in the code
** below verify that the numbers are aligned correctly.
*/
 void sqlite3ExprIfTrue(Parse *pParse, Expr *pExpr, int dest, int jumpIfNull){
  Vdbe *v = pParse.pVdbe;
  int op = 0;
  int regFree1 = 0;
  int regFree2 = 0;
  int r1, r2;

  assert( jumpIfNull==SQLITE_JUMPIFNULL || jumpIfNull==0 );
  if( v==0 )     return;  /* Existance of VDBE checked by caller */
  if( pExpr==0 ) return;  /* No way this can happen */
  op = pExpr.op;
  switch( op ){
    case TK_AND: {
      int d2 = v.MakeLabel()
      pParse.ColumnCacheContext(func() {
		  sqlite3ExprIfFalse(pParse, pExpr.pLeft, d2,jumpIfNull ^ SQLITE_JUMPIFNULL)
		  sqlite3ExprIfTrue(pParse, pExpr.pRight, dest, jumpIfNull)
		  v.ResolveLabel(d2)
      })
      break;
    }
    case TK_OR: {
      sqlite3ExprIfTrue(pParse, pExpr.pLeft, dest, jumpIfNull);
      sqlite3ExprIfTrue(pParse, pExpr.pRight, dest, jumpIfNull);
      break;
    }
    case TK_NOT: {
      sqlite3ExprIfFalse(pParse, pExpr.pLeft, dest, jumpIfNull);
      break;
    }
    case TK_LT:
    case TK_LE:
    case TK_GT:
    case TK_GE:
    case TK_NE:
    case TK_EQ: {
      assert( TK_LT==OP_Lt );
      assert( TK_LE==OP_Le );
      assert( TK_GT==OP_Gt );
      assert( TK_GE==OP_Ge );
      assert( TK_EQ==OP_Eq );
      assert( TK_NE==OP_Ne );
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, dest, jumpIfNull);
      break;
    }
    case TK_IS:
    case TK_ISNOT: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      op = (op==TK_IS) ? TK_EQ : TK_NE;
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, dest, SQLITE_NULLEQ);
      break;
    }
    case TK_ISNULL:
    case TK_NOTNULL: {
      assert( TK_ISNULL==OP_IsNull );
      assert( TK_NOTNULL==OP_NotNull );
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      v.AddOp2(op, r1, dest);
      break;
    }
    case TK_BETWEEN: {
      exprCodeBetween(pParse, pExpr, dest, 1, jumpIfNull);
      break;
    }
    case TK_IN: {
      int destIfFalse = v.MakeLabel()
      int destIfNull = jumpIfNull ? dest : destIfFalse;
      sqlite3ExprCodeIN(pParse, pExpr, destIfFalse, destIfNull);
      v.AddOp2(OP_Goto, 0, dest);
      v.ResolveLabel(destIfFalse)
      break;
    }
    default: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr, &regFree1);
      v.AddOp3(OP_If, r1, dest, jumpIfNull!=0);
      break;
    }
  }
  pParse.ReleaseTempReg(regFree1)
  pParse.ReleaseTempReg(regFree2)
}

/*
** Generate code for a boolean expression such that a jump is made
** to the label "dest" if the expression is false but execution
** continues straight thru if the expression is true.
**
** If the expression evaluates to NULL (neither true nor false) then
** jump if jumpIfNull is SQLITE_JUMPIFNULL or fall through if jumpIfNull
** is 0.
*/
 void sqlite3ExprIfFalse(Parse *pParse, Expr *pExpr, int dest, int jumpIfNull){
  Vdbe *v = pParse.pVdbe;
  int op = 0;
  int regFree1 = 0;
  int regFree2 = 0;
  int r1, r2;

  assert( jumpIfNull==SQLITE_JUMPIFNULL || jumpIfNull==0 );
  if( v==0 ) return; /* Existance of VDBE checked by caller */
  if( pExpr==0 )    return;

  /* The value of pExpr.op and op are related as follows:
  **
  **       pExpr.op            op
  **       ---------          ----------
  **       TK_ISNULL          OP_NotNull
  **       TK_NOTNULL         OP_IsNull
  **       TK_NE              OP_Eq
  **       TK_EQ              OP_Ne
  **       TK_GT              OP_Le
  **       TK_LE              OP_Gt
  **       TK_GE              OP_Lt
  **       TK_LT              OP_Ge
  **
  ** For other values of pExpr.op, op is undefined and unused.
  ** The value of TK_ and OP_ constants are arranged such that we
  ** can compute the mapping above using the following expression.
  ** Assert()s verify that the computation is correct.
  */
  op = ((pExpr.op+(TK_ISNULL&1))^1)-(TK_ISNULL&1);

  /* Verify correct alignment of TK_ and OP_ constants
  */
  assert( pExpr.op!=TK_ISNULL || op==OP_NotNull );
  assert( pExpr.op!=TK_NOTNULL || op==OP_IsNull );
  assert( pExpr.op!=TK_NE || op==OP_Eq );
  assert( pExpr.op!=TK_EQ || op==OP_Ne );
  assert( pExpr.op!=TK_LT || op==OP_Ge );
  assert( pExpr.op!=TK_LE || op==OP_Gt );
  assert( pExpr.op!=TK_GT || op==OP_Le );
  assert( pExpr.op!=TK_GE || op==OP_Lt );

  switch( pExpr.op ){
    case TK_AND: {
      sqlite3ExprIfFalse(pParse, pExpr.pLeft, dest, jumpIfNull);
      sqlite3ExprIfFalse(pParse, pExpr.pRight, dest, jumpIfNull);
      break;
    }
    case TK_OR: {
      int d2 = v.MakeLabel()
      pParse.ColumnCacheContext(func() {
		  sqlite3ExprIfTrue(pParse, pExpr.pLeft, d2, jumpIfNull ^ SQLITE_JUMPIFNULL)
    	  sqlite3ExprIfFalse(pParse, pExpr.pRight, dest, jumpIfNull)
    	  v.ResolveLabel(d2)
      })
      break
    }
    case TK_NOT: {
      sqlite3ExprIfTrue(pParse, pExpr.pLeft, dest, jumpIfNull);
      break;
    }
    case TK_LT:
    case TK_LE:
    case TK_GT:
    case TK_GE:
    case TK_NE:
    case TK_EQ: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, dest, jumpIfNull);
      break;
    }
    case TK_IS:
    case TK_ISNOT: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      r2 = sqlite3ExprCodeTemp(pParse, pExpr.pRight, &regFree2);
      op = (pExpr.op==TK_IS) ? TK_NE : TK_EQ;
      codeCompare(pParse, pExpr.pLeft, pExpr.pRight, op,
                  r1, r2, dest, SQLITE_NULLEQ);
      break;
    }
    case TK_ISNULL:
    case TK_NOTNULL: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr.pLeft, &regFree1);
      v.AddOp2(op, r1, dest);
      break;
    }
    case TK_BETWEEN: {
      exprCodeBetween(pParse, pExpr, dest, 0, jumpIfNull);
      break;
    }
    case TK_IN: {
      if( jumpIfNull ){
        sqlite3ExprCodeIN(pParse, pExpr, dest, dest);
      }else{
        int destIfNull = v.MakeLabel()
        sqlite3ExprCodeIN(pParse, pExpr, dest, destIfNull);
        v.ResolveLabel(destIfNull)
      }
      break;
    }
    default: {
      r1 = sqlite3ExprCodeTemp(pParse, pExpr, &regFree1);
      v.AddOp3(OP_IfNot, r1, dest, jumpIfNull!=0);
      break;
    }
  }
  pParse.ReleaseTempReg(regFree1)
  pParse.ReleaseTempReg(regFree2)
}

//	Do a deep comparison of two expression trees. Return 0 if the two expressions are completely identical. Return 1 if they differ only by a COLLATE operator at the top level. Return 2 if there are differences other than the top-level COLLATE operator.
//	Sometimes this routine will return 2 even if the two expressions really are equivalent. If we cannot prove that the expressions are identical, we return 2 just to be safe. So if this routine returns 2, then you do not really know for certain if the two expressions are the same. But if you get a 0 or 1 return, then you can be sure the expressions are the same. In the places where this routine is used, it does not hurt to get an extra 2 - that just might result in some slightly slower code. But returning an incorrect 0 or 1 could lead to a malfunction.
func (pA *Expr) Compare(pB *Expr) int {
	switch {
	case pA == nil || pB == nil:
		if pA != pB {
			return 2
		}
	case pA.HasProperty(EP_xIsSelect) || pB.HasProperty(EP_xIsSelect) || pA.flags & EP_Distinct != pB.flags & EP_Distinct:
		return 2
	case pA.op != pB.op || pA.pLeft.Compare(pB.pLeft) != 0 || pA.pRight.Compare(pB.pRight) != 0 || !pA.pList.Identical(pB.pList):
		return 2
	case pA.iTable != pB.iTable || pA.iColumn != pB.iColumn:
		return 2
	case pA.HasProperty(EP_IntValue) && (!pB.HasProperty(EP_IntValue) || pA.Value != pB.Value):
		return 2
	case pA.op != TK_COLUMN && pA.op != TK_AGG_COLUMN && pA.Token:
		if pB.HasProperty(EP_IntValue) || pB.Token == "" || pA.Token != pB.Token {
			return 2
		}
	case pA.flags & EP_ExpCollate != pB.flags & EP_ExpCollate:
		return 1
	case pA.flags & EP_ExpCollate != 0 && pA.pColl != pB.pColl:
		return 2
	}
	return 0
}

//	Compare two ExprList objects. Return 0 if they are identical and non-zero if they differ in any way.
//	This routine might return non-zero for equivalent ExprLists. The only consequence will be disabled optimizations. But this routine must never return 0 if the two ExprList objects are different, or a malfunction will result.
//	Two NULL pointers are considered to be the same. But a NULL pointer always differs from a non-NULL pointer.
func (pA *ExprList) Identical(pB *ExprList) bool {
int sqlite3ExprListCompare(ExprList *pA, ExprList *pB){
	switch {
	case pA == nil && pB == nil:
		return true
	case pA == nil || pB == nil:
		return false
	case pA.Len() != pB.Len():
		return false
	}
	for i, itemA := range pA.Items {
		if itemB := pB.Items[i]; itemA.sortOrder != itemB.sortOrder || itemA.Expr.Compare(itemB.Expr) != 0:
			return false
		}
	}
	return true
}

/*
** This is the expression callback for sqlite3FunctionUsesOtherSrc().
**
** Determine if an expression references any table other than one of the
** tables in pWalker.u.pSrcList and abort if it does.
*/
static int exprUsesOtherSrc(Walker *pWalker, Expr *pExpr){
  if( pExpr.op==TK_COLUMN || pExpr.op==TK_AGG_COLUMN ){
    int i;
    SrcList *pSrc = pWalker.u.pSrcList;
    for(i=0; i<pSrc.nSrc; i++){
      if( pExpr.iTable==pSrc.a[i].iCursor ) return WRC_Continue;
    }
    return WRC_Abort;
  }else{
    return WRC_Continue;
  }
}

//	Determine if any of the arguments to the pExpr Function references any SrcList other than pSrcList. Return true if they do. Return false if pExpr has no argument or has only constant arguments or only references tables named in pSrcList.
static int sqlite3FunctionUsesOtherSrc(Expr *pExpr, SrcList *pSrcList){
	Walker w;
	assert( pExpr.op==TK_AGG_FUNCTION );
	memset(&w, 0, sizeof(w));
	w.xExprCallback = exprUsesOtherSrc;
	w.u.pSrcList = pSrcList;
	if w.ExprList(pExpr.pList) != WRC_Continue {
		return 1
	}
	return 0;
}

/*
** Add a new element to the pAggInfo.Columns[] array.  Return the index of
** the new element.  Return a negative number if malloc fails.
*/
static int addAggInfoColumn(sqlite3 *db, AggInfo *pInfo){
  int i;
  pInfo.Columns = sqlite3ArrayAllocate(
       db,
       pInfo.Columns,
       sizeof(pInfo.Columns[0]),
       &pInfo.nColumn,
       &i
  );
  return i;
}    

/*
** Add a new element to the pAggInfo.aFunc[] array.  Return the index of
** the new element.  Return a negative number if malloc fails.
*/
static int addAggInfoFunc(sqlite3 *db, AggInfo *pInfo){
  int i;
  pInfo.aFunc = sqlite3ArrayAllocate(
       db, 
       pInfo.aFunc,
       sizeof(pInfo.aFunc[0]),
       &pInfo.nFunc,
       &i
  );
  return i;
}    

/*
** This is the xExprCallback for a tree walker.  It is used to
** implement sqlite3ExprAnalyzeAggregates().  See sqlite3ExprAnalyzeAggregates
** for additional information.
*/
static int analyzeAggregate(Walker *pWalker, Expr *pExpr){
  int i;
  pNC := pWalker.u.pNC
  pParse := pNC.Parse
  pSrcList := pNC.SrcList
  pAggInfo = pNC.AggInfo

  switch( pExpr.op ){
    case TK_AGG_COLUMN:
    case TK_COLUMN: {
      /* Check to see if the column is in one of the tables in the FROM
      ** clause of the aggregate query */
      if( pSrcList!=0 ){
        struct SrcList_item *pItem = pSrcList.a;
        for(i=0; i<pSrcList.nSrc; i++, pItem++){
          struct AggInfo_col *pCol;
          if( pExpr.iTable==pItem.iCursor ){
			//	If we reach this point, it means that pExpr refers to a table that is in the FROM clause of the aggregate query.
			//	Make an entry for the column in pAggInfo.Columns[] if there is not an entry there already.
            int k;
			pCol = pAggInfo.Columns
            for k = 0; k < pAggInfo.nColumn; k++, pCol++ {
				if pCol.iTable == pExpr.iTable && pCol.iColumn == pExpr.iColumn {
					break
				}
			}
			if k >= pAggInfo.nColumn {
				if k = addAggInfoColumn(pParse.db, pAggInfo); k >= 0 {
					pCol = &pAggInfo.Columns[k]
					pCol.pTab = pExpr.pTab
					pCol.iTable = pExpr.iTable
					pCol.iColumn = pExpr.iColumn
					pCol.iMem = ++pParse.nMem
					pCol.iSorterColumn = -1
					pCol.Expr = pExpr
					if pAggInfo.pGroupBy {
						for j, term := range pAggInfo.pGroupBy.Items {
							pE := term.Expr
							if pE.op == TK_COLUMN && pE.iTable == pExpr.iTable && pE.iColumn == pExpr.iColumn {
								pCol.iSorterColumn = j
								break
							}
						}
					}
					if pCol.iSorterColumn < 0 {
						pAggInfo.nSortingColumn++
						pCol.iSorterColumn = pAggInfo.nSortingColumn
					}
				}
			}
			//	There is now an entry for pExpr in pAggInfo.Columns[] (either because it was there before or because we just created it). Convert the pExpr to be a TK_AGG_COLUMN referring to that pAggInfo.Columns[] entry.
			pExpr.pAggInfo = pAggInfo
			pExpr.op = TK_AGG_COLUMN
			pExpr.iAgg = int16(k)
			break
          } /* endif pExpr.iTable==pItem.iCursor */
        } /* end loop over pSrcList */
      }
      return WRC_Prune;
    }
    case TK_AGG_FUNCTION: {
      if( (pNC.Flags & NC_InAggFunc)==0
       && !sqlite3FunctionUsesOtherSrc(pExpr, pSrcList)
      ){
        /* Check to see if pExpr is a duplicate of another aggregate 
        ** function that is already in the pAggInfo structure
        */
        struct AggInfo_func *pItem = pAggInfo.aFunc;
        for(i=0; i<pAggInfo.nFunc; i++, pItem++){
          if pItem.Expr.Compare(pExpr) == 0 {
            break
          }
        }
        if( i>=pAggInfo.nFunc ){
          /* pExpr is original.  Make a new entry in pAggInfo.aFunc[]
          */
          enc := pParse.db.Encoding()
          i = addAggInfoFunc(pParse.db, pAggInfo);
          if( i>=0 ){
            assert( !pExpr.HasProperty(EP_xIsSelect) );
            pItem = &pAggInfo.aFunc[i];
            pItem.Expr = pExpr;
            pItem.iMem = ++pParse.nMem;
            assert( !pExpr.HasProperty(EP_IntValue) );
			if pExpr.pList != nil {
				pItem.pFunc = pParse.db.FindFunction(pExpr.Token, pExpr.pList.Len(), enc, false)
			} else {
				pItem.pFunc = pParse.db.FindFunction(pExpr.Token, 0, enc, false)
			}
            if( pExpr.flags & EP_Distinct ){
              pItem.iDistinct = pParse.nTab++;
            }else{
              pItem.iDistinct = -1;
            }
          }
        }
        //	Make pExpr point to the appropriate pAggInfo.aFunc[] entry
        pExpr.iAgg = int16(i)
        pExpr.pAggInfo = pAggInfo
      }
      return WRC_Prune;
    }
  }
  return WRC_Continue;
}
static int analyzeAggregatesInSelect(Walker *pWalker, Select *pSelect){
  return WRC_Continue;
}

/*
** Analyze the given expression looking for aggregate functions and
** for variables that need to be added to the pParse.aAgg[] array.
** Make additional entries to the pParse.aAgg[] array as necessary.
**
** This routine should only be called after the expression has been
** analyzed by sqlite3ResolveExprNames().
*/
 void sqlite3ExprAnalyzeAggregates(NameContext *pNC, Expr *pExpr){
  Walker w;
  memset(&w, 0, sizeof(w));
  w.xExprCallback = analyzeAggregate;
  w.xSelectCallback = analyzeAggregatesInSelect;
  w.u.pNC = pNC;
  assert( pNC.SrcList != nil );
  w.Expr(pExpr)
}

//	Call sqlite3ExprAnalyzeAggregates() for every expression in an expression list. Return the number of errors.
//	If an error is found, the analysis is cut short.
func (pNC *NameContext) ExprAnalyzeAggList(pList *ExprList) {
	if pList != nil {
		for i, item := range pList.Items {
			sqlite3ExprAnalyzeAggregates(pNC, item.Expr)
		}
	}
}

//	Allocate a single new register for use to hold some intermediate result.
func (pParse *Parse) GetTempReg() (register int) {
	if len(pParse.aTempReg) == 0 {
		pParse.nMem++
		register = pParse.nMem
	} else {
		i := len(pParse.aTempReg) - 1
		register = pParse.aTempReg[i]
		pParse.aTempReg = pParse.aTempReg[:i]
	}
}

//	Deallocate a register, making available for reuse for some other purpose.
//	If a register is currently being used by the column cache, then the deallocation is deferred until the column cache line that uses the register becomes stale.
func (pParse *Parse) ReleaseTempReg(register int) {
	if register != 0 {
		for i, p := range pParse.ColumnsCache {
			if p.iReg == register {
				p.needsFreeing = true
				return
			}
		}
		pParse.aTempReg = append(pParse.aTempReg, register)
	}
}

func (pParse *Parse) GetTempRange(count int) (i int) {
	i = pParse.iRangeReg
	if n := pParse.nRangeReg; count <= n {
		assert( !usedAsColumnCache(pParse, i, i + n - 1) )
		pParse.iRangeReg += count
		pParse.nRangeReg -= count
	} else {
		i = pParse.nMem + 1
		pParse.nMem += count
	}
	return
}

func (pParse *Parse) ReleaseTempRange(register, count int) {
	pParse.ExprCacheRemove(register, count)
	if count > pParse.nRangeReg {
		pParse.nRangeReg = count
		pParse.iRangeReg = register
	}
}

func (pParse *Parse) ClearTempRegCache() {
	pParse.aTempReg = pParse.aTempReg[:0]
	pParse.nRangeReg = 0
}