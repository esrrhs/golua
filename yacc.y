%{
package fakego

import "github.com/esrrhs/go-engine/src/loggo"

%}

%union {
  s string
  sn syntree_node
}

%token VAR_BEGIN;
%token RETURN;
%token BREAK;
%token FUNC;
%token WHILE;
%token FTRUE;
%token FFALSE;
%token IF;
%token THEN;
%token ELSE;
%token END;
%token STRING_DEFINITION;
%token IDENTIFIER;
%token NUMBER;
%token SINGLE_LINE_COMMENT;
%token DIVIDE_MOD;
%token ARG_SPLITTER;
%token PLUS;
%token MINUS;
%token DIVIDE;
%token MULTIPLY;
%token ASSIGN;
%token MORE;
%token LESS;
%token MORE_OR_EQUAL;
%token LESS_OR_EQUAL;
%token EQUAL;
%token NOT_EQUAL;
%token OPEN_BRACKET;
%token CLOSE_BRACKET;
%token AND;
%token OR;
%token FKFLOAT;
%token PLUS_ASSIGN;
%token MINUS_ASSIGN;
%token DIVIDE_ASSIGN;
%token MULTIPLY_ASSIGN;
%token DIVIDE_MOD_ASSIGN;
%token COLON;
%token FOR;
%token INC;
%token FAKE;
%token FKUUID;
%token OPEN_SQUARE_BRACKET;
%token CLOSE_SQUARE_BRACKET;
%token FCONST;
%token PACKAGE;
%token INCLUDE;
%token IDENTIFIER_DOT;
%token IDENTIFIER_POINTER;
%token STRUCT;
%token IS;
%token NOT; 
%token CONTINUE;
%token YIELD;
%token SLEEP;
%token SWITCH;
%token CASE; 
%token DEFAULT;
%token NEW_ASSIGN;
%token ELSEIF;
%token RIGHT_POINTER;
%token STRING_CAT;
%token OPEN_BIG_BRACKET;
%token CLOSE_BIG_BRACKET;
%token FNULL;

%left PLUS MINUS
%left DIVIDE MULTIPLY DIVIDE_MOD
%left STRING_CAT

%%
  

/* Top level rules */
program: package_head
	include_head
	struct_head
	const_head 
	body 
	;
	
package_head:
	/* empty */
	{
	}
	|
	PACKAGE IDENTIFIER
	{
		loggo.Debug("[yacc]: package %v", $2.s);
		l := yylex.(lexerwarpper).mf
		l.packageName = ($2.s);
	}
	|
	PACKAGE IDENTIFIER_DOT
	{
		loggo.Debug("[yacc]: package %v", $2.s);
		l := yylex.(lexerwarpper).mf
		l.packageName = ($2.s);
	}
	
include_head:
	/* empty */
	{
	}
	|
	include_define
	|
	include_head include_define
	;
	
include_define:
	INCLUDE STRING_DEFINITION
	{
		loggo.Debug("[yacc]: include %v", $2.s);
		l := yylex.(lexerwarpper).mf
		l.add_include($2.s);
	}
	;

struct_head:
	/* empty */
	{
	}
	|
	struct_define
	|
	struct_head struct_define
	;

struct_define:
	STRUCT IDENTIFIER struct_mem_declaration END
	{
		loggo.Debug("[yacc]: struct_define %v", $2.s);
		l := yylex.(lexerwarpper).mf
		if ($3.sn) != nil {
			p := ($3.sn).(*struct_desc_memlist_node)
			l.add_struct_desc($2.s, p)
		}
	}
	;
	
struct_mem_declaration: 
	/* empty */
	{
		$$.sn = nil
	}
	| 
	struct_mem_declaration IDENTIFIER 
	{
		loggo.Debug("[yacc]: struct_mem_declaration <- IDENTIFIER struct_mem_declaration");
		p := ($1.sn).(*struct_desc_memlist_node)
		p.add_arg($2.s)
		$$.sn = p
	}
	| 
	IDENTIFIER
	{
		loggo.Debug("[yacc]: struct_mem_declaration <- IDENTIFIER");
		p := &struct_desc_memlist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.add_arg($1.s)
		$$.sn = p
	}
	;

const_head:
	/* empty */
	{
	}
	|
	const_define
	|
	const_head const_define
	;

const_define:
	FCONST IDENTIFIER ASSIGN explicit_value
	{
		loggo.Debug("[yacc]: const_define %v", $2.s);
		l := yylex.(lexerwarpper).mf
		l.add_const_desc($2.s, $4.sn)
	}
	;

body:
	/* empty */
	{
	}
	|
	function_declaration
	|
	body function_declaration
	;

/* function declaration begin */

function_declaration:
	FUNC IDENTIFIER OPEN_BRACKET function_declaration_arguments CLOSE_BRACKET block END
	{
		loggo.Debug("[yacc]: function_declaration <- block %v", $2.s);
		p := &func_desc_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.funcname = $2.s
		p.arglist = $4.sn.(*func_desc_arglist_node)
		p.block = $6.sn.(*block_node)
		l := yylex.(lexerwarpper).mf
		l.add_func_desc(p)
	}
	|
	FUNC IDENTIFIER OPEN_BRACKET function_declaration_arguments CLOSE_BRACKET END
	{
		loggo.Debug("[yacc]: function_declaration <- empty %v", $2.s);
		p := &func_desc_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.funcname = $2.s
                p.arglist = $4.sn.(*func_desc_arglist_node)
		p.block = nil
		l := yylex.(lexerwarpper).mf
		l.add_func_desc(p)
	}
	;

function_declaration_arguments: 
	/* empty */
	{
		$$.sn = nil
	}
	| 
	function_declaration_arguments ARG_SPLITTER arg 
	{
		loggo.Debug("[yacc]: function_declaration_arguments <- arg function_declaration_arguments");
		p := ($1.sn).(*func_desc_arglist_node)
		p.add_arg($3.s)
		$$.sn = p
	}
	| 
	arg
	{
		loggo.Debug("[yacc]: function_declaration_arguments <- arg");
		p := &func_desc_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.add_arg($1.s)
		$$.sn = p
	}
	;

arg : 
	IDENTIFIER
	{
		loggo.Debug("[yacc]: arg <- IDENTIFIER %v", $1.s);
		p := &identifier_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.str = $1.s
		$$.sn = p
	}
	;
	
function_call:
	IDENTIFIER OPEN_BRACKET function_call_arguments CLOSE_BRACKET 
	{
		loggo.Debug("[yacc]: function_call <- function_call_arguments %v", $1.s);
		p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.fuc = $1.s
		p.prefunc = nil
		p.arglist = ($3.sn).(*function_call_arglist_node)
		p.fakecall = false
		p.classmem_call = false
		$$.sn = p
	} 
	|
	IDENTIFIER_DOT OPEN_BRACKET function_call_arguments CLOSE_BRACKET 
	{
		loggo.Debug("[yacc]: function_call <- function_call_arguments %v", $1.s);
		p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.fuc = $1.s
		p.prefunc = nil
		p.arglist = ($3.sn).(*function_call_arglist_node)
		p.fakecall = false
		p.classmem_call = false
		$$.sn = p
	} 
	|
	function_call OPEN_BRACKET function_call_arguments CLOSE_BRACKET 
	{
		loggo.Debug("[yacc]: function_call <- function_call_arguments");
		p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.fuc = ""
		p.prefunc = $1.sn;
		p.arglist = ($3.sn).(*function_call_arglist_node)
		p.fakecall = false
		p.classmem_call = false
		$$.sn = p
	} 
	|
	function_call COLON IDENTIFIER OPEN_BRACKET function_call_arguments CLOSE_BRACKET 
	{
		loggo.Debug("[yacc]: function_call <- mem function_call_arguments %v", $3.s);
		p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.fuc = $3.s
		p.prefunc = nil
		if $5.sn == nil {
			p.arglist = &function_call_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		} else {
			p.arglist = ($5.sn).(*function_call_arglist_node)
		}
		p.arglist.add_arg($1.sn)
		p.fakecall = false
		p.classmem_call = true
		$$.sn = p
	}
	|
	variable COLON IDENTIFIER OPEN_BRACKET function_call_arguments CLOSE_BRACKET 
	{
		loggo.Debug("[yacc]: function_call <- mem function_call_arguments %v", $3.s);
		p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		p.fuc = $3.s
		p.prefunc = nil
		if $5.sn == nil {
			p.arglist = &function_call_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
		} else {
			p.arglist = ($5.sn).(*function_call_arglist_node)
		}
		p.arglist.add_arg($1.sn)
		p.fakecall = false
		p.classmem_call = true
		$$.sn = p
	} 
	;
	
function_call_arguments: 
	/* empty */
	{
		//$$ = 0;
	}
	| 
	function_call_arguments ARG_SPLITTER arg_expr
	{
		loggo.Debug("[yacc]: function_call_arguments <- arg_expr function_call_arguments");
		//assert($1->gettype() == est_call_arglist);
		//function_call_arglist_node * p = dynamic_cast<function_call_arglist_node*>($1);
		//p->add_arg($3);
		//$$ = p;
	}
	| 
	arg_expr
	{
		loggo.Debug("[yacc]: function_call_arguments <- arg_expr");
		//NEWTYPE(p, function_call_arglist_node);
		//p->add_arg($1);
		//$$ = p;
	}
	;  

arg_expr:
	expr_value
	{
		loggo.Debug("[yacc]: arg_expr <- expr_value");
		//$$ = $1;
	}
	;

/* function declaration end */

block:
	block stmt 
	{
		loggo.Debug("[yacc]: block <- block stmt");
		//assert($1->gettype() == est_block);
		//block_node * p = dynamic_cast<block_node*>($1);
		//p->add_stmt($2);
		//$$ = p;
	}
	|
	stmt 
	{
		loggo.Debug("[yacc]: block <- stmt");
		//NEWTYPE(p, block_node);
		//p->add_stmt($1);
		//$$ = p;
	}
	;
  
stmt:
	while_stmt
	{
		loggo.Debug("[yacc]: stmt <- while_stmt");
		//$$ = $1;
	}
	|
	if_stmt
	{
		loggo.Debug("[yacc]: stmt <- if_stmt");
		//$$ = $1;
	}
	|
	return_stmt
	{
		loggo.Debug("[yacc]: stmt <- return_stmt");
		//$$ = $1;
	}
	|
	assign_stmt
	{
		loggo.Debug("[yacc]: stmt <- assign_stmt");
		//$$ = $1;
	}
	|
	multi_assign_stmt
	{
		loggo.Debug("[yacc]: stmt <- multi_assign_stmt");
		//$$ = $1;
	}
	|
	break
	{
		loggo.Debug("[yacc]: stmt <- break");
		//$$ = $1;
	}
	|
	continue
	{
		loggo.Debug("[yacc]: stmt <- continue");
		//$$ = $1;
	}
	|
	expr
	{
		loggo.Debug("[yacc]: stmt <- expr");
		//$$ = $1;
	}
	|
	math_assign_stmt
	{
		loggo.Debug("[yacc]: stmt <- math_assign_stmt");
		//$$ = $1;
	}
	|
	for_stmt
	{
		loggo.Debug("[yacc]: stmt <- for_stmt");
		//$$ = $1;
	}
	|
	for_loop_stmt
	{
		loggo.Debug("[yacc]: stmt <- for_loop_stmt");
		//$$ = $1;
	}
	|
	fake_call_stmt
	{
		loggo.Debug("[yacc]: stmt <- fake_call_stmt");
		//$$ = $1;
	}
	|
	sleep
	{
		loggo.Debug("[yacc]: stmt <- sleep_stmt");
		//$$ = $1;
	}
	|
	yield
	{
		loggo.Debug("[yacc]: stmt <- yield_stmt");
		//$$ = $1;
	}
	|
	switch_stmt
	{
		loggo.Debug("[yacc]: stmt <- switch_stmt");
		//$$ = $1;
	}
	;

fake_call_stmt:
	FAKE function_call
	{
		loggo.Debug("[yacc]: fake_call_stmt <- fake function_call");
		//function_call_node * p = dynamic_cast<function_call_node*>($2);
		//p->fakecall = true;
		//$$ = p;
	}
	;
	
for_stmt:
	FOR block ARG_SPLITTER cmp ARG_SPLITTER block THEN block END
	{
		loggo.Debug("[yacc]: for_stmt <- block cmp block");
		//NEWTYPE(p, for_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($4);
		//p->beginblock = dynamic_cast<block_node*>($2);
		//p->endblock = dynamic_cast<block_node*>($6);
		//p->block = dynamic_cast<block_node*>($8);
		//$$ = p;
	}
	|
	FOR block ARG_SPLITTER cmp ARG_SPLITTER block THEN END
	{
		loggo.Debug("[yacc]: for_stmt <- block cmp");
		//NEWTYPE(p, for_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($4);
		//p->beginblock = dynamic_cast<block_node*>($2);
		//p->endblock = dynamic_cast<block_node*>($6);
		//p->block = 0;
		//$$ = p;
	}
	;

for_loop_stmt:
	FOR var ASSIGN assign_value RIGHT_POINTER cmp_value ARG_SPLITTER expr_value THEN block END
	{
		loggo.Debug("[yacc]: for_loop_stmt <- block");
		//NEWTYPE(p, for_stmt);
		//
		//syntree_node * pi = $2;
		//if (pi->gettype() == est_var)
		//{
		//	NEWTYPE(pvar, variable_node);
		//	pvar->str = (dynamic_cast<var_node*>(pi))->str;
		//	pi = pvar;
		//}
		//
		//NEWTYPE(pcmp, cmp_stmt);
		//pcmp->cmp = "<";
		//pcmp->left = pi;
		//pcmp->right = $6;
		//p->cmp = pcmp;
		//
		//NEWTYPE(pbeginblockassign, assign_stmt);
		//pbeginblockassign->var = $2;
		//pbeginblockassign->value = $4;
		//pbeginblockassign->isnew = false;
		//NEWTYPE(pbeginblock, block_node);
		//pbeginblock->add_stmt(pbeginblockassign);
		//p->beginblock = pbeginblock;
		//
		//NEWTYPE(pendblockassign, math_assign_stmt);
		//pendblockassign->var = pi;
		//pendblockassign->oper = "+=";
		//pendblockassign->value = $8;
		//NEWTYPE(pendblock, block_node);
		//pendblock->add_stmt(pendblockassign);
		//p->endblock = pendblock;
		//
		//p->block = dynamic_cast<block_node*>($10);
		//$$ = p;
	}
	|
	FOR var ASSIGN assign_value RIGHT_POINTER cmp_value ARG_SPLITTER expr_value THEN END
	{
		loggo.Debug("[yacc]: for_loop_stmt <- empty");
		//NEWTYPE(p, for_stmt);
		//
		//NEWTYPE(pcmp, cmp_stmt);
		//pcmp->cmp = "<";
		//pcmp->left = $2;
		//pcmp->right = $6;
		//p->cmp = pcmp;
		//
		//NEWTYPE(pbeginblockassign, assign_stmt);
		//pbeginblockassign->var = $2;
		//pbeginblockassign->value = $4;
		//pbeginblockassign->isnew = false;
		//NEWTYPE(pbeginblock, block_node);
		//pbeginblock->add_stmt(pbeginblockassign);
		//p->beginblock = pbeginblock;
		//
		//NEWTYPE(pendblockassign, math_assign_stmt);
		//pendblockassign->var = $2;
		//pendblockassign->oper = "+=";
		//pendblockassign->value = $8;
		//NEWTYPE(pendblock, block_node);
		//pendblock->add_stmt(pendblockassign);
		//p->endblock = pendblock;
		//
		//p->block = 0;
		//$$ = p;
	}
	;
	
while_stmt:
	WHILE cmp THEN block END 
	{
		loggo.Debug("[yacc]: while_stmt <- cmp block");
		//NEWTYPE(p, while_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = dynamic_cast<block_node*>($4);
		//$$ = p;
	}
	|
	WHILE cmp THEN END 
	{
		loggo.Debug("[yacc]: while_stmt <- cmp");
		//NEWTYPE(p, while_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = 0;
		//$$ = p;
	}
	;
	
if_stmt:
	IF cmp THEN block elseif_stmt_list else_stmt END
	{
		loggo.Debug("[yacc]: if_stmt <- cmp block");
		//NEWTYPE(p, if_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = dynamic_cast<block_node*>($4);
		//p->elseifs = dynamic_cast<elseif_stmt_list*>($5);
		//p->elses = dynamic_cast<else_stmt*>($6);
		//$$ = p;
	}
	|
	IF cmp THEN elseif_stmt_list else_stmt END
	{
		loggo.Debug("[yacc]: if_stmt <- cmp");
		//NEWTYPE(p, if_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = 0;
		//p->elseifs = dynamic_cast<elseif_stmt_list*>($4);
		//p->elses = dynamic_cast<else_stmt*>($5);
		//$$ = p;
	}
	;
	
elseif_stmt_list:
	/* empty */
	{
		//$$ = 0;
	}
	| 
	elseif_stmt_list elseif_stmt
	{
		loggo.Debug("[yacc]: elseif_stmt_list <- elseif_stmt_list elseif_stmt");
		//assert($1->gettype() == est_elseif_stmt_list);
		//elseif_stmt_list * p = dynamic_cast<elseif_stmt_list*>($1);
		//p->add_stmt($2);
		//$$ = p;
	}
	| 
	elseif_stmt
	{
		loggo.Debug("[yacc]: elseif_stmt_list <- elseif_stmt");
		//NEWTYPE(p, elseif_stmt_list);
		//p->add_stmt($1);
		//$$ = p;
	}
	;
	
elseif_stmt:
	ELSEIF cmp THEN block
	{
		loggo.Debug("[yacc]: elseif_stmt <- ELSEIF cmp THEN block");
		//NEWTYPE(p, elseif_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = $4;
		//$$ = p;
	}
	|
	ELSEIF cmp THEN
	{
		loggo.Debug("[yacc]: elseif_stmt <- ELSEIF cmp THEN block");
		//NEWTYPE(p, elseif_stmt);
		//p->cmp = dynamic_cast<cmp_stmt*>($2);
		//p->block = 0;
		//$$ = p;
	}
	;
	
else_stmt:
	/*empty*/
	{
		//$$ = 0;
	}
	|
	ELSE block
	{
		loggo.Debug("[yacc]: else_stmt <- block");
		//NEWTYPE(p, else_stmt);
		//p->block = dynamic_cast<block_node*>($2);
		//$$ = p;
	}
	|
	ELSE
	{
		loggo.Debug("[yacc]: else_stmt <- empty");
		//NEWTYPE(p, else_stmt);
		//p->block = 0;
		//$$ = p;
	}
	;

cmp:
	OPEN_BRACKET cmp CLOSE_BRACKET
	{
		loggo.Debug("[yacc]: cmp <- ( cmp )");
		//$$ = $2;
	}
	|
	cmp AND cmp
	{
		loggo.Debug("[yacc]: cmp <- cmp AND cmp");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "&&";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp OR cmp
	{
		loggo.Debug("[yacc]: cmp <- cmp OR cmp");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "||";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value LESS cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value LESS cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value MORE cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value MORE cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value EQUAL cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value EQUAL cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value MORE_OR_EQUAL cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value MORE_OR_EQUAL cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value LESS_OR_EQUAL cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value LESS_OR_EQUAL cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	cmp_value NOT_EQUAL cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value NOT_EQUAL cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = $2;
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	FTRUE
	{
		loggo.Debug("[yacc]: cmp <- true");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "true";
		//p->left = 0;
		//p->right = 0;
		//$$ = p;
	}
	|
	FFALSE
	{
		loggo.Debug("[yacc]: cmp <- false");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "false";
		//p->left = 0;
		//p->right = 0;
		//$$ = p;
	}
	|
	IS cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value IS cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "is";
		//p->left = $2;
		//p->right = 0;
		//$$ = p;
	}
	|
	NOT cmp_value
	{
		loggo.Debug("[yacc]: cmp <- cmp_value NOT cmp_value");
		//NEWTYPE(p, cmp_stmt);
		//p->cmp = "not";
		//p->left = $2;
		//p->right = 0;
		//$$ = p;
	}
	;

cmp_value:
	explicit_value
	{
		loggo.Debug("[yacc]: cmp_value <- explicit_value");
		//$$ = $1;
	}
	|
	variable
	{
		loggo.Debug("[yacc]: cmp_value <- variable");
		//$$ = $1;
	}
	|
	expr
	{
		loggo.Debug("[yacc]: cmp_value <- expr");
		//$$ = $1;
	}
	;
	
return_stmt:
	RETURN return_value_list
	{
		loggo.Debug("[yacc]: return_stmt <- RETURN return_value_list");
		//NEWTYPE(p, return_stmt);
		//p->returnlist = dynamic_cast<return_value_list_node*>($2);
		//$$ = p;
	}
	|
	RETURN
	{
		loggo.Debug("[yacc]: return_stmt <- RETURN");
		//NEWTYPE(p, return_stmt);
		//p->returnlist = 0;
		//$$ = p;
	}
	;
 
return_value_list:
	return_value_list ARG_SPLITTER return_value
	{
		loggo.Debug("[yacc]: return_value_list <- return_value_list return_value");
		//assert($1->gettype() == est_return_value_list);
		//return_value_list_node * p = dynamic_cast<return_value_list_node*>($1);
		//p->add_arg($3);
		//$$ = p;
	}
	|
	return_value
	{
		loggo.Debug("[yacc]: return_value_list <- return_value");
		//NEWTYPE(p, return_value_list_node);
		//p->add_arg($1);
		//$$ = p;
	}
	;
 
return_value:
	explicit_value
	{
		loggo.Debug("[yacc]: return_value <- explicit_value");
		//$$ = $1;
	}
	|
	variable
	{
		loggo.Debug("[yacc]: return_value <- variable");
		//$$ = $1;
	}
	|
	expr
	{
		loggo.Debug("[yacc]: return_value <- expr");
		//$$ = $1;
	}
	;

assign_stmt:
	var ASSIGN assign_value
	{
		loggo.Debug("[yacc]: assign_stmt <- var assign_value");
		//NEWTYPE(p, assign_stmt);
		//p->var = $1;
		//p->value = $3;
		//p->isnew = false;
		//$$ = p;
	}
	|
	var NEW_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: new assign_stmt <- var assign_value");
		//NEWTYPE(p, assign_stmt);
		//p->var = $1;
		//p->value = $3;
		//p->isnew = true;
		//$$ = p;
	}
	;

multi_assign_stmt:
	var_list ASSIGN function_call
	{
		loggo.Debug("[yacc]: multi_assign_stmt <- var_list function_call");
		//NEWTYPE(p, multi_assign_stmt);
		//p->varlist = dynamic_cast<var_list_node*>($1);
		//p->value = $3;
		//p->isnew = false;
		//$$ = p;
	}
	|
	var_list NEW_ASSIGN function_call
	{
		loggo.Debug("[yacc]: new multi_assign_stmt <- var_list function_call");
		//NEWTYPE(p, multi_assign_stmt);
		//p->varlist = dynamic_cast<var_list_node*>($1);
		//p->value = $3;
		//p->isnew = true;
		//$$ = p;
	}
	;
	
var_list:
	var_list ARG_SPLITTER var
	{
		loggo.Debug("[yacc]: var_list <- var_list var");
		//assert($1->gettype() == est_var_list);
		//var_list_node * p = dynamic_cast<var_list_node*>($1);
		//p->add_arg($3);
		//$$ = p;
	}
	|
	var
	{
		loggo.Debug("[yacc]: var_list <- var");
		//NEWTYPE(p, var_list_node);
		//p->add_arg($1);
		//$$ = p;
	}
	;
	
assign_value:
	explicit_value
	{
		loggo.Debug("[yacc]: assign_value <- explicit_value");
		//$$ = $1;
	}
	|
	variable
	{
		loggo.Debug("[yacc]: assign_value <- variable");
		//$$ = $1;
	}
	|
	expr
	{
		loggo.Debug("[yacc]: assign_value <- expr");
		//$$ = $1;
	}
	;
	
math_assign_stmt :
	variable PLUS_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value");
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "+=";
		//p->value = $3;
		//$$ = p;
	}
	|
	variable MINUS_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value");
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "-=";
		//p->value = $3;
		//$$ = p;
	}
	|
	variable DIVIDE_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value");
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "/=";
		//p->value = $3;
		//$$ = p;
	}
	|
	variable MULTIPLY_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value");
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "*=";
		//p->value = $3;
		//$$ = p;
	}
	|
	variable DIVIDE_MOD_ASSIGN assign_value
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value");
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "%=";
		//p->value = $3;
		//$$ = p;
	}
	|
	variable INC
	{
		loggo.Debug("[yacc]: math_assign_stmt <- variable INC");
		//NEWTYPE(pp, explicit_value_node);
		//pp->str = "1";
		//pp->type = explicit_value_node::EVT_NUM;
		//
		//NEWTYPE(p, math_assign_stmt);
		//p->var = $1;
		//p->oper = "+=";
		//p->value = pp;
		//$$ = p;
	}
	;
	
var:
	VAR_BEGIN IDENTIFIER
	{
		loggo.Debug("[yacc]: var <- VAR_BEGIN IDENTIFIER %v", $2.s);
		//NEWTYPE(p, var_node);
		//p->str = $2;
		//$$ = p;
	}
	|
	variable
	{
		loggo.Debug("[yacc]: var <- variable");
		//$$ = $1;
	}
	;

variable:
	IDENTIFIER
	{
		loggo.Debug("[yacc]: variable <- IDENTIFIER %v", $1.s);
		//NEWTYPE(p, variable_node);
		//p->str = $1;
		//$$ = p;
	}
	|
	IDENTIFIER OPEN_SQUARE_BRACKET expr_value CLOSE_SQUARE_BRACKET
	{
		loggo.Debug("[yacc]: container_get_node <- IDENTIFIER[expr_value] %v", $1.s);
		//NEWTYPE(p, container_get_node);
		//p->container = $1;
		//p->key = $3;
		//$$ = p;
	}
	|
	IDENTIFIER_POINTER
	{
		loggo.Debug("[yacc]: variable <- IDENTIFIER_POINTER %v", $1.s);
		//NEWTYPE(p, struct_pointer_node);
		//p->str = $1;
		//$$ = p;
	}
	|
	IDENTIFIER_DOT
	{
		loggo.Debug("[yacc]: variable <- IDENTIFIER_DOT %v", $1.s);
		//NEWTYPE(p, variable_node);
		//p->str = $1;
		//$$ = p;
	}
	;

expr:
	OPEN_BRACKET expr CLOSE_BRACKET
	{
		loggo.Debug("[yacc]: expr <- (expr)");
		//$$ = $2;
	}
	|
	function_call
	{
		loggo.Debug("[yacc]: expr <- function_call");
		//$$ = $1;
	}
	|
	math_expr
	{
		loggo.Debug("[yacc]: expr <- math_expr");
		//$$ = $1;
	}
	;

math_expr:
	OPEN_BRACKET math_expr CLOSE_BRACKET
	{
		loggo.Debug("[yacc]: math_expr <- (math_expr)");
		//$$ = $2;
	}
	|
	expr_value PLUS expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "+";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	expr_value MINUS expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "-";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	expr_value MULTIPLY expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "*";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	expr_value DIVIDE expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "/";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	expr_value DIVIDE_MOD expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "%";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	|
	expr_value STRING_CAT expr_value
	{
		loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", $2.s);
		//NEWTYPE(p, math_expr_node);
		//p->oper = "..";
		//p->left = $1;
		//p->right = $3;
		//$$ = p;
	}
	;	

expr_value:
	math_expr
	{
		loggo.Debug("[yacc]: expr_value <- math_expr");
		//$$ = $1;
	}
	|
	explicit_value
	{
		loggo.Debug("[yacc]: expr_value <- explicit_value");
		//$$ = $1;
	}
	|
	function_call
	{
		loggo.Debug("[yacc]: expr_value <- function_call");
		//$$ = $1;
	}
	|
	variable
	{
		loggo.Debug("[yacc]: expr_value <- variable");
		//$$ = $1;
	}
	;
	
explicit_value:
	FTRUE 
	{
		loggo.Debug("[yacc]: explicit_value <- FTRUE");
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_TRUE;
		//$$ = p;
	}
	|
	FFALSE 
	{
		loggo.Debug("[yacc]: explicit_value <- FFALSE");
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_FALSE;
		//$$ = p;
	}
	|
	NUMBER 
	{
		loggo.Debug("[yacc]: explicit_value <- NUMBER %v", $1.s);
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_NUM;
		//$$ = p;
	}
	|
	FKUUID
	{
		loggo.Debug("[yacc]: explicit_value <- FKUUID %v", $1.s);
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_UUID;
		//$$ = p;
	}
	|
	STRING_DEFINITION 
	{
		loggo.Debug("[yacc]: explicit_value <- STRING_DEFINITION %v", $1.s);
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_STR;
		//$$ = p;
	}
	|
	FKFLOAT
	{
		loggo.Debug("[yacc]: explicit_value <- FKFLOAT %v", $1.s);
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_FLOAT;
		//$$ = p;
	}
	|
	FNULL
	{
		loggo.Debug("[yacc]: explicit_value <- FNULL %v", $1.s);
		//NEWTYPE(p, explicit_value_node);
		//p->str = $1;
		//p->type = explicit_value_node::EVT_NULL;
		//$$ = p;
	}
	|
	OPEN_BIG_BRACKET const_map_list_value CLOSE_BIG_BRACKET
	{
		loggo.Debug("[yacc]: explicit_value <- const_map_list_value");
		//NEWTYPE(p, explicit_value_node);
		//p->str = "";
		//p->type = explicit_value_node::EVT_MAP;
		//p->v = $2;
		//$$ = p;
	}
	|
	OPEN_SQUARE_BRACKET const_array_list_value CLOSE_SQUARE_BRACKET
	{
		loggo.Debug("[yacc]: explicit_value <- const_array_list_value");
		//NEWTYPE(p, explicit_value_node);
		//p->str = "";
		//p->type = explicit_value_node::EVT_ARRAY;
		//p->v = $2;
		//$$ = p;
	}
	;
      
const_map_list_value:
	/* empty */
	{
		loggo.Debug("[yacc]: const_map_list_value <- null");
		//NEWTYPE(p, const_map_list_value_node);
		//$$ = p;
	}
	|
	const_map_value
	{
		loggo.Debug("[yacc]: const_map_list_value <- const_map_value");
		//NEWTYPE(p, const_map_list_value_node);
		//p->add_ele($1);
		//$$ = p;
	}
	|
	const_map_list_value const_map_value
	{
		loggo.Debug("[yacc]: const_map_list_value <- const_map_list_value const_map_value");
		//assert($1->gettype() == est_constmaplist);
		//const_map_list_value_node * p = dynamic_cast<const_map_list_value_node*>($1);
		//p->add_ele($2);
		//$$ = p;
	}
	;
	
const_map_value:
	explicit_value COLON explicit_value
	{
		loggo.Debug("[yacc]: const_map_value <- explicit_value");
		//NEWTYPE(p, const_map_value_node);
		//p->k = $1;
		//p->v = $3;
		//$$ = p;
	}
	;

const_array_list_value:
	/* empty */
	{
		loggo.Debug("[yacc]: const_array_list_value <- null");
		//NEWTYPE(p, const_array_list_value_node);
		//$$ = p;
	}
	|
	explicit_value
	{
		loggo.Debug("[yacc]: const_array_list_value <- explicit_value");
		//NEWTYPE(p, const_array_list_value_node);
		//p->add_ele($1);
		//$$ = p;
	}
	|
	const_array_list_value explicit_value
	{
		loggo.Debug("[yacc]: const_array_list_value <- const_array_list_value explicit_value");
		//assert($1->gettype() == est_constarraylist);
		//const_array_list_value_node * p = dynamic_cast<const_array_list_value_node*>($1);
		//p->add_ele($2);
		//$$ = p;
	}
	;
	
break:
	BREAK 
	{
		loggo.Debug("[yacc]: break <- BREAK");
		//NEWTYPE(p, break_stmt);
		//$$ = p;
	}
	;
	
continue:
	CONTINUE 
	{
		loggo.Debug("[yacc]: CONTINUE");
		//NEWTYPE(p, continue_stmt);
		//$$ = p;
	}
	;

sleep:
	SLEEP expr_value 
	{
		loggo.Debug("[yacc]: SLEEP");
		//NEWTYPE(p, sleep_stmt);
		//p->time = $2;
		//$$ = p;
	}
	
yield:
	YIELD expr_value
	{
		loggo.Debug("[yacc]: YIELD");
		//NEWTYPE(p, yield_stmt);
		//p->time = $2;
		//$$ = p;
	}
	;
	
switch_stmt:
	SWITCH cmp_value switch_case_list DEFAULT block END
	{
		loggo.Debug("[yacc]: switch_stmt");
		//NEWTYPE(p, switch_stmt);
		//p->cmp = $2;
		//p->caselist = $3;
		//p->def = $5;
		//$$ = p;
	}
	|
	SWITCH cmp_value switch_case_list DEFAULT END
	{
		loggo.Debug("[yacc]: switch_stmt");
		//NEWTYPE(p, switch_stmt);
		//p->cmp = $2;
		//p->caselist = $3;
		//p->def = 0;
		//$$ = p;
	}
	;
	
switch_case_list:
	switch_case_define
	{
		loggo.Debug("[yacc]: switch_case_list <- switch_case_define");
		//NEWTYPE(p, switch_caselist_node);
		//p->add_case($1);
		//$$ = p;
	}
	|
	switch_case_list switch_case_define
	{
		loggo.Debug("[yacc]: switch_case_list <- switch_case_list switch_case_define");
		//assert($2->gettype() == est_switch_case_node);
		//switch_caselist_node * p = dynamic_cast<switch_caselist_node*>($1);
		//p->add_case($2);
		//$$ = p;
	}
	;

switch_case_define:
	CASE cmp_value THEN block
	{
		loggo.Debug("[yacc]: switch_case_define");
		//NEWTYPE(p, switch_case_node);
		//p->cmp = $2;
		//p->block = $4;
		//$$ = p;
	}
	|
	CASE cmp_value THEN
	{
		loggo.Debug("[yacc]: switch_case_define");
		//NEWTYPE(p, switch_case_node);
		//p->cmp = $2;
		//p->block = 0;
		//$$ = p;
	}
	;
	
%%

func init() {
	yyErrorVerbose = true // set the global that enables showing full errors
}