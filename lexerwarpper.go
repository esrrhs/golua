package fake

type lexerwarpper struct {
	yyLexer
	mf myflexer
}
