/* This file contains code used to insert the values of host parameters
** (aka "wildcards") into the SQL text output by sqlite3_trace().
**
** The Vdbe parse-tree explainer is also found here.
*/

#ifndef SQLITE_OMIT_TRACE

/*
** zSql is a zero-terminated string of UTF-8 SQL text.  Return the number of
** bytes in this text up to but excluding the first character in
** a host parameter.  If the text contains no host parameters, return
** the total number of bytes in the text.
*/
static int findNextHostParameter(const char *zSql, int *pnToken){
  int tokenType;
  int nTotal = 0;
  int n;

  *pnToken = 0;
  while( zSql[0] ){
    n = sqlite3GetToken((byte*)zSql, &tokenType);
    assert( n>0 && tokenType!=TK_ILLEGAL );
    if( tokenType==TK_VARIABLE ){
      *pnToken = n;
      break;
    }
    nTotal += n;
    zSql += n;
  }
  return nTotal;
}

//	This function returns a pointer to a nul-terminated string in memory obtained from sqlite3DbMalloc(). If sqlite3.vdbeExecCnt is 1, then the string contains a copy of zRawSql but with host parameters expanded to their current bindings. Or, if sqlite3.vdbeExecCnt is greater than 1, then the returned string holds a copy of zRawSql with "-- " prepended to each line of text.
//	The calling function is responsible for making sure the memory returned is eventually freed.
//	ALGORITHM:  Scan the input string looking for host parameters in any of these forms:  ?, ?N, $A, @A, :A. Take care to avoid text within string literals, quoted identifier names, and comments. For text forms, the host parameter index is found by scanning the perpared statement for the corresponding OP_Variable opcode. Once the host parameter index is known, locate the value in p.aVar[]. Then render the value as a literal in place of the host parameter name.
func (p *Vdbe) ExpandSql(zRawSql string) string {
	idx := 0				//	Index of a host parameter
	nextIndex := 1			//	Index of next ? host parameter
	db := p.db;
	out := ""
	if db.vdbeExecCnt > 1 {
		for zRawSql != nil {
			zStart = zRawSql
			for *(zRawSql++) != '\n' && zRawSql != nil ) {
			}
			out = append(out, "-- ", zStart)			//	len(zStart) == int(zRawSql - zStart)
		}
	} else {
		n := 0				//	Length of a token prefix
		nToken := 0			//	Length of the parameter token
		for zRawSql[0] != "" {
			n = findNextHostParameter(zRawSql, &nToken)
			assert( n > 0 )
			out = append(out, zRawSql)
			zRawSql += n
			assert( zRawSql[0] || nToken == 0 )
			if nToken == 0 {
				break
			}
			if zRawSql[0] == '?' {
				if nToken > 1 {
					assert( sqlite3Isdigit(zRawSql[1]) )
					sqlite3GetInt32(&zRawSql[1], &idx)
				} else {
					idx = nextIndex
				}
			} else {
				assert( zRawSql[0] == ':' || zRawSql[0] == '$' || zRawSql[0] == '@' )
				idx = sqlite3VdbeParameterIndex(p, zRawSql, nToken)
				assert( idx > 0 )
			}
			zRawSql += nToken
			nextIndex = idx + 1
			assert( idx > 0 && idx <= p.nVar )
			parameter := p.aVar[idx - 1]
			if parameter.Value == nil {
				out = append(out, "NULL")
			} else if v, ok := parameter.Value.(int64) {
				out = fmt.Sprintf("%lld", v)
			} else if parameter.flags & MEM_Real {
				out = fmt.Sprintf("%!.15g", parameter.r)
			} else if parameter.flags & MEM_Str {
#ifndef SQLITE_OMIT_UTF16
				byte enc = db.Encoding()
				if enc != SQLITE_UTF8 {
					utf8 := &Mem{ db: db }
					utf8.SetStr(parameter.z, enc, SQLITE_STATIC)
					sqlite3VdbeChangeEncoding(utf8, SQLITE_UTF8)
					out = fmt.Sprintf("'%.*q'", utf8.n, utf8.z)
					utf8.Release()
				} else
#endif
				out = fmt.Sprintf("'%.*q'", parameter.n, parameter.z);
			} else if v, ok := parameter.Value.(Zeroes); ok {
				out = fmt.Sprintf("zeroblob(%d)", v)
			} else {
				assert( parameter.flags & MEM_Blob )
				out = append(out, "x'")
				for i := 0; i < len(parameter.z); i++ {
					out = append(out, fmt.Sprintf("%02x", parameter.z[i] & 0xff))
				}
				out = append(out, "'")
			}
		}
	}
	return out
}

#endif /* #ifndef SQLITE_OMIT_TRACE */