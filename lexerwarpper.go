package fakego

type lexerwarpper struct {
	yyLexer
	mf myflexer
}
