//line ../yacc.y:1
package fakego

import __yyfmt__ "fmt"

//line ../yacc.y:3
import "github.com/esrrhs/go-engine/src/loggo"

//line ../yacc.y:8
type yySymType struct {
	yys int
	s   string
	sn  syntree_node
}

const VAR_BEGIN = 57346
const RETURN = 57347
const BREAK = 57348
const FUNC = 57349
const WHILE = 57350
const FTRUE = 57351
const FFALSE = 57352
const IF = 57353
const THEN = 57354
const ELSE = 57355
const END = 57356
const STRING_DEFINITION = 57357
const IDENTIFIER = 57358
const NUMBER = 57359
const SINGLE_LINE_COMMENT = 57360
const DIVIDE_MOD = 57361
const ARG_SPLITTER = 57362
const PLUS = 57363
const MINUS = 57364
const DIVIDE = 57365
const MULTIPLY = 57366
const ASSIGN = 57367
const MORE = 57368
const LESS = 57369
const MORE_OR_EQUAL = 57370
const LESS_OR_EQUAL = 57371
const EQUAL = 57372
const NOT_EQUAL = 57373
const OPEN_BRACKET = 57374
const CLOSE_BRACKET = 57375
const AND = 57376
const OR = 57377
const FKFLOAT = 57378
const PLUS_ASSIGN = 57379
const MINUS_ASSIGN = 57380
const DIVIDE_ASSIGN = 57381
const MULTIPLY_ASSIGN = 57382
const DIVIDE_MOD_ASSIGN = 57383
const COLON = 57384
const FOR = 57385
const INC = 57386
const FAKE = 57387
const FKUUID = 57388
const OPEN_SQUARE_BRACKET = 57389
const CLOSE_SQUARE_BRACKET = 57390
const FCONST = 57391
const PACKAGE = 57392
const INCLUDE = 57393
const IDENTIFIER_DOT = 57394
const IDENTIFIER_POINTER = 57395
const STRUCT = 57396
const IS = 57397
const NOT = 57398
const CONTINUE = 57399
const YIELD = 57400
const SLEEP = 57401
const SWITCH = 57402
const CASE = 57403
const DEFAULT = 57404
const NEW_ASSIGN = 57405
const ELSEIF = 57406
const RIGHT_POINTER = 57407
const STRING_CAT = 57408
const OPEN_BIG_BRACKET = 57409
const CLOSE_BIG_BRACKET = 57410
const FNULL = 57411

var yyToknames = [...]string{
	"$end",
	"error",
	"$unk",
	"VAR_BEGIN",
	"RETURN",
	"BREAK",
	"FUNC",
	"WHILE",
	"FTRUE",
	"FFALSE",
	"IF",
	"THEN",
	"ELSE",
	"END",
	"STRING_DEFINITION",
	"IDENTIFIER",
	"NUMBER",
	"SINGLE_LINE_COMMENT",
	"DIVIDE_MOD",
	"ARG_SPLITTER",
	"PLUS",
	"MINUS",
	"DIVIDE",
	"MULTIPLY",
	"ASSIGN",
	"MORE",
	"LESS",
	"MORE_OR_EQUAL",
	"LESS_OR_EQUAL",
	"EQUAL",
	"NOT_EQUAL",
	"OPEN_BRACKET",
	"CLOSE_BRACKET",
	"AND",
	"OR",
	"FKFLOAT",
	"PLUS_ASSIGN",
	"MINUS_ASSIGN",
	"DIVIDE_ASSIGN",
	"MULTIPLY_ASSIGN",
	"DIVIDE_MOD_ASSIGN",
	"COLON",
	"FOR",
	"INC",
	"FAKE",
	"FKUUID",
	"OPEN_SQUARE_BRACKET",
	"CLOSE_SQUARE_BRACKET",
	"FCONST",
	"PACKAGE",
	"INCLUDE",
	"IDENTIFIER_DOT",
	"IDENTIFIER_POINTER",
	"STRUCT",
	"IS",
	"NOT",
	"CONTINUE",
	"YIELD",
	"SLEEP",
	"SWITCH",
	"CASE",
	"DEFAULT",
	"NEW_ASSIGN",
	"ELSEIF",
	"RIGHT_POINTER",
	"STRING_CAT",
	"OPEN_BIG_BRACKET",
	"CLOSE_BIG_BRACKET",
	"FNULL",
}
var yyStatenames = [...]string{}

const yyEofCode = 1
const yyErrCode = 2
const yyInitialStackSize = 16

//line ../yacc.y:1459

func init() {
	yyErrorVerbose = true // set the global that enables showing full errors
}

//line yacctab:1
var yyExca = [...]int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 83,
	19, 128,
	21, 128,
	22, 128,
	23, 128,
	24, 128,
	66, 128,
	-2, 117,
	-1, 84,
	19, 126,
	21, 126,
	22, 126,
	23, 126,
	24, 126,
	66, 126,
	-2, 118,
	-1, 85,
	20, 111,
	25, 111,
	63, 111,
	-2, 129,
	-1, 104,
	12, 81,
	20, 81,
	33, 81,
	34, 81,
	35, 81,
	-2, 130,
	-1, 105,
	12, 82,
	20, 82,
	33, 82,
	34, 82,
	35, 82,
	-2, 131,
	-1, 108,
	19, 127,
	21, 127,
	22, 127,
	23, 127,
	24, 127,
	66, 127,
	-2, 85,
	-1, 109,
	19, 129,
	21, 129,
	22, 129,
	23, 129,
	24, 129,
	66, 129,
	-2, 86,
	-1, 114,
	19, 127,
	21, 127,
	22, 127,
	23, 127,
	24, 127,
	66, 127,
	-2, 92,
	-1, 115,
	19, 129,
	21, 129,
	22, 129,
	23, 129,
	24, 129,
	66, 129,
	-2, 93,
	-1, 123,
	26, 118,
	27, 118,
	28, 118,
	29, 118,
	30, 118,
	31, 118,
	-2, 126,
	-1, 170,
	19, 127,
	21, 127,
	22, 127,
	23, 127,
	24, 127,
	66, 127,
	-2, 101,
	-1, 171,
	19, 129,
	21, 129,
	22, 129,
	23, 129,
	24, 129,
	66, 129,
	-2, 102,
}

const yyPrivate = 57344

const yyLast = 1228

var yyAct = [...]int{

	85, 219, 113, 196, 237, 78, 169, 183, 220, 153,
	152, 239, 148, 149, 151, 150, 245, 267, 197, 229,
	193, 101, 182, 197, 152, 17, 148, 149, 151, 150,
	12, 91, 77, 80, 6, 75, 33, 34, 76, 233,
	3, 270, 37, 92, 35, 43, 7, 6, 152, 146,
	12, 59, 151, 150, 127, 83, 121, 153, 118, 82,
	46, 119, 221, 38, 152, 54, 148, 149, 151, 150,
	86, 153, 87, 36, 41, 103, 109, 109, 115, 93,
	95, 22, 8, 124, 81, 89, 88, 90, 137, 124,
	124, 109, 135, 181, 40, 153, 39, 97, 111, 120,
	145, 226, 117, 109, 125, 225, 53, 109, 109, 100,
	262, 153, 155, 156, 126, 146, 180, 147, 171, 171,
	137, 137, 177, 17, 157, 173, 124, 176, 31, 171,
	171, 171, 171, 171, 28, 187, 188, 189, 190, 191,
	118, 84, 124, 136, 140, 140, 124, 124, 124, 124,
	124, 124, 124, 124, 124, 92, 109, 109, 244, 186,
	109, 109, 109, 109, 109, 109, 143, 252, 198, 115,
	200, 222, 155, 156, 224, 174, 175, 209, 210, 57,
	168, 140, 165, 166, 185, 144, 100, 254, 44, 155,
	156, 93, 95, 109, 171, 167, 27, 140, 109, 230,
	228, 140, 140, 140, 140, 140, 140, 140, 140, 140,
	211, 155, 156, 21, 227, 25, 224, 155, 156, 23,
	236, 224, 109, 96, 123, 124, 124, 124, 238, 253,
	139, 139, 241, 26, 234, 212, 213, 214, 215, 216,
	217, 249, 224, 240, 123, 238, 109, 94, 242, 243,
	18, 13, 32, 154, 259, 232, 91, 250, 224, 100,
	29, 248, 30, 124, 47, 49, 134, 139, 178, 47,
	100, 223, 56, 231, 22, 155, 156, 195, 98, 48,
	140, 140, 140, 194, 51, 45, 79, 139, 139, 139,
	139, 139, 139, 139, 139, 139, 16, 50, 100, 108,
	108, 114, 112, 100, 179, 95, 11, 100, 5, 74,
	100, 20, 100, 10, 108, 100, 15, 73, 140, 72,
	100, 256, 71, 70, 69, 68, 108, 66, 65, 64,
	108, 108, 63, 62, 207, 67, 138, 142, 61, 60,
	42, 170, 170, 24, 19, 14, 9, 218, 4, 2,
	1, 0, 170, 170, 170, 170, 170, 128, 129, 130,
	131, 132, 127, 0, 133, 0, 139, 139, 139, 0,
	0, 0, 0, 184, 0, 0, 0, 0, 0, 108,
	108, 0, 0, 108, 108, 108, 108, 108, 108, 0,
	0, 0, 114, 184, 199, 184, 201, 202, 203, 204,
	205, 206, 33, 34, 139, 0, 0, 0, 37, 246,
	35, 110, 110, 116, 0, 0, 108, 170, 122, 251,
	0, 108, 0, 0, 255, 0, 110, 0, 258, 38,
	0, 0, 260, 0, 0, 0, 0, 0, 158, 36,
	41, 263, 110, 110, 0, 108, 0, 268, 0, 0,
	0, 0, 0, 172, 172, 0, 0, 0, 0, 0,
	40, 52, 39, 0, 172, 172, 172, 172, 172, 108,
	0, 0, 184, 184, 184, 0, 0, 0, 104, 105,
	0, 0, 0, 0, 37, 92, 35, 0, 0, 0,
	0, 110, 110, 0, 0, 110, 110, 110, 110, 110,
	110, 102, 0, 0, 116, 38, 0, 91, 77, 80,
	265, 75, 33, 34, 76, 36, 41, 269, 37, 92,
	35, 93, 95, 0, 106, 107, 0, 0, 110, 172,
	0, 0, 0, 110, 0, 82, 40, 0, 39, 38,
	160, 159, 162, 163, 161, 164, 86, 0, 87, 36,
	41, 0, 0, 0, 0, 93, 95, 110, 0, 0,
	81, 89, 88, 90, 0, 0, 0, 0, 0, 0,
	40, 0, 39, 91, 77, 80, 0, 75, 33, 34,
	76, 110, 0, 266, 37, 92, 35, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 33, 34, 0, 0,
	0, 82, 37, 0, 35, 38, 0, 0, 0, 0,
	0, 0, 86, 0, 87, 36, 41, 0, 0, 0,
	0, 93, 95, 38, 0, 0, 81, 89, 88, 90,
	0, 0, 0, 36, 41, 55, 40, 0, 39, 91,
	77, 80, 0, 75, 33, 34, 76, 0, 0, 264,
	37, 92, 35, 0, 40, 0, 39, 91, 77, 80,
	0, 75, 33, 34, 76, 261, 0, 82, 37, 92,
	35, 38, 0, 0, 0, 0, 0, 0, 86, 0,
	87, 36, 41, 0, 0, 82, 0, 93, 95, 38,
	0, 0, 81, 89, 88, 90, 86, 0, 87, 36,
	41, 0, 40, 0, 39, 93, 95, 0, 0, 0,
	81, 89, 88, 90, 0, 0, 0, 0, 0, 0,
	40, 0, 39, 91, 77, 80, 0, 75, 33, 34,
	76, 0, 0, 257, 37, 92, 35, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 33, 34, 0, 0,
	0, 82, 37, 0, 35, 38, 0, 0, 0, 0,
	0, 0, 86, 0, 87, 36, 41, 0, 0, 0,
	0, 93, 95, 38, 0, 0, 81, 89, 88, 90,
	0, 0, 0, 36, 41, 0, 40, 0, 39, 91,
	77, 80, 0, 75, 33, 34, 76, 0, 0, 247,
	37, 92, 35, 0, 40, 0, 39, 91, 77, 80,
	0, 75, 33, 34, 76, 0, 0, 82, 37, 92,
	35, 38, 0, 0, 0, 0, 0, 0, 86, 0,
	87, 36, 41, 0, 0, 82, 0, 93, 95, 38,
	0, 0, 81, 89, 88, 90, 86, 0, 87, 36,
	41, 0, 40, 0, 39, 93, 95, 0, 0, 0,
	81, 89, 88, 90, 0, 0, 0, 221, 0, 0,
	40, 0, 39, 91, 77, 80, 0, 75, 33, 34,
	76, 0, 0, 235, 37, 92, 35, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 82, 0, 0, 0, 38, 0, 0, 0, 0,
	0, 0, 86, 0, 87, 36, 41, 0, 0, 0,
	0, 93, 95, 0, 0, 0, 81, 89, 88, 90,
	0, 0, 0, 0, 0, 0, 40, 0, 39, 91,
	77, 80, 0, 75, 33, 34, 76, 0, 0, 208,
	37, 92, 35, 0, 0, 0, 0, 91, 77, 80,
	0, 75, 33, 34, 76, 0, 0, 82, 37, 92,
	35, 38, 0, 192, 0, 0, 0, 0, 86, 0,
	87, 36, 41, 0, 0, 82, 0, 93, 95, 38,
	0, 0, 81, 89, 88, 90, 86, 0, 87, 36,
	41, 0, 40, 0, 39, 93, 95, 0, 0, 0,
	81, 89, 88, 90, 0, 0, 0, 0, 0, 0,
	40, 0, 39, 91, 77, 80, 0, 75, 33, 34,
	76, 0, 0, 99, 37, 92, 35, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 82, 0, 0, 0, 38, 0, 0, 0, 0,
	0, 0, 86, 0, 87, 36, 41, 0, 0, 0,
	0, 93, 95, 0, 0, 0, 81, 89, 88, 90,
	0, 0, 0, 0, 0, 0, 40, 0, 39, 91,
	77, 80, 0, 75, 33, 34, 76, 0, 0, 58,
	37, 92, 35, 0, 0, 0, 0, 91, 77, 80,
	0, 75, 33, 34, 76, 0, 0, 82, 37, 92,
	35, 38, 0, 0, 0, 0, 0, 0, 86, 0,
	87, 36, 41, 0, 0, 82, 0, 93, 95, 38,
	0, 0, 81, 89, 88, 90, 86, 0, 87, 36,
	41, 0, 40, 0, 39, 93, 95, 0, 33, 34,
	81, 89, 88, 90, 37, 92, 35, 33, 34, 0,
	40, 0, 39, 37, 92, 35, 0, 0, 0, 0,
	0, 141, 0, 0, 0, 38, 0, 0, 0, 0,
	82, 0, 0, 0, 38, 36, 41, 0, 0, 0,
	0, 93, 95, 0, 36, 41, 0, 0, 0, 0,
	93, 95, 0, 0, 0, 0, 40, 0, 39, 0,
	0, 0, 0, 0, 0, 40, 0, 39,
}
var yyPact = [...]int{

	-10, -1000, -17, 30, -4, -1000, 236, -1000, -1000, -24,
	-1000, -1000, 234, -1000, 74, -1000, -1000, 203, 199, 267,
	-1000, -1000, 180, 109, 246, -1000, -1000, 96, 737, -1000,
	-1000, 172, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000,
	737, 737, 264, -1000, -1000, 393, -1000, 23, 587, -1000,
	1085, 172, -1000, -1000, 737, -1000, -1000, 1019, -1000, -1000,
	-1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000, -1000,
	-1000, -1000, -1000, -1000, -1000, 469, 469, 1158, 77, 36,
	-1000, -1000, 1158, 72, -1000, 320, 1103, 139, 1149, 1149,
	1158, 169, 68, 85, 45, -1000, -1000, -1000, -1000, -1000,
	-1000, 241, 469, 514, -1000, -1000, 1158, 1158, -1000, 12,
	-1000, 183, 160, -1000, -1000, 12, -1000, 1158, 1158, 139,
	139, 252, 83, 60, 12, 1149, 168, 143, 1158, 1158,
	1158, 1158, 1158, -1000, 953, -5, 72, 12, 45, -1000,
	72, 1149, 45, -38, -1000, 1149, 1149, 1149, 1149, 1149,
	1149, 1149, 1149, 1149, 935, 469, 469, 177, 83, 1158,
	1158, 1158, 1158, 1158, 1158, -1000, -1000, 803, 1158, -1000,
	-1000, 12, -1000, -1000, 72, 72, -1000, -1000, 2, -1000,
	-1000, -1000, 238, -1000, 45, 73, 69, -1000, -1000, -1000,
	-1000, -1000, 469, 1158, 60, -43, -1000, 1158, 222, -9,
	201, 29, 29, -57, -57, -57, -1000, 869, -1000, 78,
	78, -1000, -1000, -1000, -1000, -1000, -1000, -1000, 803, -2,
	-1000, 469, -1000, -1000, 1149, 1149, 1149, 138, -49, 785,
	-1000, 249, -1000, -1000, -1000, -1000, -2, 243, -1000, 1103,
	155, -1000, 196, 154, 1103, 1158, 719, -1000, 1103, 240,
	-1000, 1103, 1103, -1000, -1000, 653, 90, -1000, 1103, -1000,
	1103, 635, 1149, 569, -1000, 5, -1000, 503, 27, -1000,
	-1000,
}
var yyPgo = [...]int{

	0, 350, 349, 348, 346, 345, 344, 308, 306, 343,
	296, 223, 213, 340, 179, 45, 55, 22, 0, 7,
	247, 51, 339, 338, 333, 332, 329, 328, 327, 335,
	325, 324, 323, 322, 319, 317, 309, 21, 5, 6,
	75, 1, 4, 8, 302, 2, 286, 141, 285, 279,
	60, 277, 3,
}
var yyR1 = [...]int{

	0, 1, 2, 2, 2, 3, 3, 3, 7, 4,
	4, 4, 8, 9, 9, 9, 5, 5, 5, 10,
	6, 6, 6, 12, 12, 13, 13, 13, 15, 16,
	16, 16, 16, 16, 17, 17, 17, 19, 14, 14,
	21, 21, 21, 21, 21, 21, 21, 21, 21, 21,
	21, 21, 21, 21, 21, 33, 31, 31, 32, 32,
	22, 22, 23, 23, 41, 41, 41, 43, 43, 42,
	42, 42, 37, 37, 37, 37, 37, 37, 37, 37,
	37, 37, 37, 37, 37, 40, 40, 40, 24, 24,
	44, 44, 45, 45, 45, 25, 25, 26, 26, 46,
	46, 39, 39, 39, 30, 30, 30, 30, 30, 30,
	38, 38, 18, 18, 18, 18, 29, 29, 29, 47,
	47, 47, 47, 47, 47, 47, 20, 20, 20, 20,
	11, 11, 11, 11, 11, 11, 11, 11, 11, 48,
	48, 48, 50, 49, 49, 49, 27, 28, 34, 35,
	36, 36, 51, 51, 52, 52,
}
var yyR2 = [...]int{

	0, 5, 0, 2, 2, 0, 1, 2, 2, 0,
	1, 2, 4, 0, 2, 1, 0, 1, 2, 4,
	0, 1, 2, 7, 6, 0, 3, 1, 1, 4,
	4, 4, 6, 6, 0, 3, 1, 1, 2, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 2, 9, 8, 11, 10,
	5, 4, 7, 6, 0, 2, 1, 4, 3, 0,
	2, 1, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 1, 1, 2, 2, 1, 1, 1, 2, 1,
	3, 1, 1, 1, 1, 3, 3, 3, 3, 3,
	1, 1, 1, 1, 3, 3, 3, 3, 3, 2,
	2, 1, 1, 4, 1, 1, 3, 1, 1, 3,
	3, 3, 3, 3, 3, 3, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 3, 3, 0,
	1, 2, 3, 0, 1, 2, 1, 1, 2, 2,
	6, 5, 1, 2, 4, 3,
}
var yyChk = [...]int{

	-1000, -1, -2, 50, -3, -7, 51, 16, 52, -4,
	-7, -8, 54, 15, -5, -8, -10, 49, 16, -6,
	-10, -12, 7, 16, -9, 16, -12, 16, 25, 14,
	16, 32, -11, 9, 10, 17, 46, 15, 36, 69,
	67, 47, -13, -15, 16, -48, -50, -11, -49, -11,
	33, 20, 68, -50, 42, 48, -11, -14, 14, -21,
	-22, -23, -24, -25, -26, -27, -28, -29, -30, -31,
	-32, -33, -34, -35, -36, 8, 11, 5, -38, -46,
	6, 57, 32, -16, -47, -18, 43, 45, 59, 58,
	60, 4, 16, 52, -20, 53, -11, -15, -11, 14,
	-21, -37, 32, -40, 9, 10, 55, 56, -11, -18,
	-29, -37, -44, -45, -11, -18, -29, 25, 63, 25,
	63, 20, -29, -47, -18, 32, 42, 42, 37, 38,
	39, 40, 41, 44, -14, -38, -16, -18, -20, -47,
	-16, 32, -20, -40, 16, 32, 47, 32, 21, 22,
	24, 23, 19, 66, 12, 34, 35, -37, -29, 27,
	26, 30, 28, 29, 31, -40, -40, 12, 20, -39,
	-11, -18, -29, -39, -16, -16, -38, -18, 16, 52,
	33, 33, -17, -19, -20, 16, 16, -39, -39, -39,
	-39, -39, 20, 25, -47, -51, -52, 61, -17, -20,
	-17, -20, -20, -20, -20, -20, -20, -14, 14, -37,
	-37, 33, -40, -40, -40, -40, -40, -40, -14, -41,
	-43, 64, -45, 33, 20, 32, 32, -37, -39, 62,
	-52, -40, 33, 48, 33, 14, -41, -42, -43, 13,
	-37, -19, -17, -17, 20, 65, -14, 14, 12, -42,
	14, -14, 12, 33, 33, -14, -40, 14, -14, 14,
	-14, 12, 20, -14, 14, -20, 14, 12, -14, 14,
	14,
}
var yyDef = [...]int{

	2, -2, 5, 0, 9, 6, 0, 3, 4, 16,
	7, 10, 0, 8, 20, 11, 17, 0, 13, 1,
	18, 21, 0, 0, 0, 15, 22, 0, 0, 12,
	14, 25, 19, 130, 131, 132, 133, 134, 135, 136,
	139, 143, 0, 27, 28, 0, 140, 0, 0, 144,
	0, 0, 137, 141, 0, 138, 145, 0, 24, 39,
	40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
	50, 51, 52, 53, 54, 0, 0, 89, 100, 0,
	146, 147, 0, -2, -2, -2, 0, 0, 0, 0,
	0, 0, 112, 115, 0, 114, 127, 26, 142, 23,
	38, 0, 0, 0, -2, -2, 0, 0, -2, -2,
	87, 0, 88, 91, -2, -2, 94, 0, 0, 0,
	0, 0, 0, -2, 129, 34, 0, 0, 0, 0,
	0, 0, 0, 109, 0, 100, 55, 0, 148, 126,
	128, 0, 149, 0, 110, 34, 0, 34, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 87, 0,
	0, 0, 0, 0, 0, 83, 84, 64, 0, 95,
	-2, -2, 103, 96, 97, 98, 99, 111, 112, 115,
	116, 119, 0, 36, 37, 0, 0, 104, 105, 106,
	107, 108, 0, 0, 126, 0, 152, 0, 0, 0,
	0, 120, 121, 122, 123, 124, 125, 0, 61, 73,
	74, 72, 75, 76, 77, 78, 79, 80, 64, 69,
	66, 0, 90, 31, 0, 34, 34, 0, 95, 0,
	153, 0, 29, 113, 30, 60, 69, 0, 65, 71,
	0, 35, 0, 0, 0, 0, 0, 151, 155, 0,
	63, 70, 68, 32, 33, 0, 0, 150, 154, 62,
	67, 0, 0, 0, 57, 0, 56, 0, 0, 59,
	58,
}
var yyTok1 = [...]int{

	1,
}
var yyTok2 = [...]int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	32, 33, 34, 35, 36, 37, 38, 39, 40, 41,
	42, 43, 44, 45, 46, 47, 48, 49, 50, 51,
	52, 53, 54, 55, 56, 57, 58, 59, 60, 61,
	62, 63, 64, 65, 66, 67, 68, 69,
}
var yyTok3 = [...]int{
	0,
}

var yyErrorMessages = [...]struct {
	state int
	token int
	msg   string
}{}

//line yaccpar:1

/*	parser for yacc output	*/

var (
	yyDebug        = 0
	yyErrorVerbose = false
)

type yyLexer interface {
	Lex(lval *yySymType) int
	Error(s string)
}

type yyParser interface {
	Parse(yyLexer) int
	Lookahead() int
}

type yyParserImpl struct {
	lval  yySymType
	stack [yyInitialStackSize]yySymType
	char  int
}

func (p *yyParserImpl) Lookahead() int {
	return p.char
}

func yyNewParser() yyParser {
	return &yyParserImpl{}
}

const yyFlag = -1000

func yyTokname(c int) string {
	if c >= 1 && c-1 < len(yyToknames) {
		if yyToknames[c-1] != "" {
			return yyToknames[c-1]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func yyErrorMessage(state, lookAhead int) string {
	const TOKSTART = 4

	if !yyErrorVerbose {
		return "syntax error"
	}

	for _, e := range yyErrorMessages {
		if e.state == state && e.token == lookAhead {
			return "syntax error: " + e.msg
		}
	}

	res := "syntax error: unexpected " + yyTokname(lookAhead)

	// To match Bison, suggest at most four expected tokens.
	expected := make([]int, 0, 4)

	// Look for shiftable tokens.
	base := yyPact[state]
	for tok := TOKSTART; tok-1 < len(yyToknames); tok++ {
		if n := base + tok; n >= 0 && n < yyLast && yyChk[yyAct[n]] == tok {
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}
	}

	if yyDef[state] == -2 {
		i := 0
		for yyExca[i] != -1 || yyExca[i+1] != state {
			i += 2
		}

		// Look for tokens that we accept or reduce.
		for i += 2; yyExca[i] >= 0; i += 2 {
			tok := yyExca[i]
			if tok < TOKSTART || yyExca[i+1] == 0 {
				continue
			}
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}

		// If the default action is to accept or reduce, give up.
		if yyExca[i+1] != 0 {
			return res
		}
	}

	for i, tok := range expected {
		if i == 0 {
			res += ", expecting "
		} else {
			res += " or "
		}
		res += yyTokname(tok)
	}
	return res
}

func yylex1(lex yyLexer, lval *yySymType) (char, token int) {
	token = 0
	char = lex.Lex(lval)
	if char <= 0 {
		token = yyTok1[0]
		goto out
	}
	if char < len(yyTok1) {
		token = yyTok1[char]
		goto out
	}
	if char >= yyPrivate {
		if char < yyPrivate+len(yyTok2) {
			token = yyTok2[char-yyPrivate]
			goto out
		}
	}
	for i := 0; i < len(yyTok3); i += 2 {
		token = yyTok3[i+0]
		if token == char {
			token = yyTok3[i+1]
			goto out
		}
	}

out:
	if token == 0 {
		token = yyTok2[1] /* unknown char */
	}
	if yyDebug >= 3 {
		__yyfmt__.Printf("lex %s(%d)\n", yyTokname(token), uint(char))
	}
	return char, token
}

func yyParse(yylex yyLexer) int {
	return yyNewParser().Parse(yylex)
}

func (yyrcvr *yyParserImpl) Parse(yylex yyLexer) int {
	var yyn int
	var yyVAL yySymType
	var yyDollar []yySymType
	_ = yyDollar // silence set and not used
	yyS := yyrcvr.stack[:]

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	yystate := 0
	yyrcvr.char = -1
	yytoken := -1 // yyrcvr.char translated into internal numbering
	defer func() {
		// Make sure we report no lookahead when not parsing.
		yystate = -1
		yyrcvr.char = -1
		yytoken = -1
	}()
	yyp := -1
	goto yystack

ret0:
	return 0

ret1:
	return 1

yystack:
	/* put a state and value onto the stack */
	if yyDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", yyTokname(yytoken), yyStatname(yystate))
	}

	yyp++
	if yyp >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyS[yyp] = yyVAL
	yyS[yyp].yys = yystate

yynewstate:
	yyn = yyPact[yystate]
	if yyn <= yyFlag {
		goto yydefault /* simple state */
	}
	if yyrcvr.char < 0 {
		yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
	}
	yyn += yytoken
	if yyn < 0 || yyn >= yyLast {
		goto yydefault
	}
	yyn = yyAct[yyn]
	if yyChk[yyn] == yytoken { /* valid shift */
		yyrcvr.char = -1
		yytoken = -1
		yyVAL = yyrcvr.lval
		yystate = yyn
		if Errflag > 0 {
			Errflag--
		}
		goto yystack
	}

yydefault:
	/* default state action */
	yyn = yyDef[yystate]
	if yyn == -2 {
		if yyrcvr.char < 0 {
			yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
		}

		/* look through exception table */
		xi := 0
		for {
			if yyExca[xi+0] == -1 && yyExca[xi+1] == yystate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			yyn = yyExca[xi+0]
			if yyn < 0 || yyn == yytoken {
				break
			}
		}
		yyn = yyExca[xi+1]
		if yyn < 0 {
			goto ret0
		}
	}
	if yyn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			yylex.Error(yyErrorMessage(yystate, yytoken))
			Nerrs++
			if yyDebug >= 1 {
				__yyfmt__.Printf("%s", yyStatname(yystate))
				__yyfmt__.Printf(" saw %s\n", yyTokname(yytoken))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for yyp >= 0 {
				yyn = yyPact[yyS[yyp].yys] + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = yyAct[yyn] /* simulate a shift of "error" */
					if yyChk[yystate] == yyErrCode {
						goto yystack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if yyDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", yyTokname(yytoken))
			}
			if yytoken == yyEofCode {
				goto ret1
			}
			yyrcvr.char = -1
			yytoken = -1
			goto yynewstate /* try again in the same state */
		}
	}

	/* reduction by production yyn */
	if yyDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
	}

	yynt := yyn
	yypt := yyp
	_ = yypt // guard against "declared and not used"

	yyp -= yyR2[yyn]
	// yyp is now the index of $0. Perform the default action. Iff the
	// reduced production is ε, $1 is possibly out of range.
	if yyp+1 >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyVAL = yyS[yyp+1]

	/* consult goto table to find next state */
	yyn = yyR1[yyn]
	yyg := yyPgo[yyn]
	yyj := yyg + yyS[yyp].yys + 1

	if yyj >= yyLast {
		yystate = yyAct[yyg]
	} else {
		yystate = yyAct[yyj]
		if yyChk[yystate] != -yyn {
			yystate = yyAct[yyg]
		}
	}
	// dummy call; replaced with literal code
	switch yynt {

	case 2:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:97
		{
		}
	case 3:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:101
		{
			loggo.Debug("[yacc]: package %v", yyDollar[2].s)
			l := yylex.(lexerwarpper).mf
			l.packageName = (yyDollar[2].s)
		}
	case 4:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:108
		{
			loggo.Debug("[yacc]: package %v", yyDollar[2].s)
			l := yylex.(lexerwarpper).mf
			l.packageName = (yyDollar[2].s)
		}
	case 5:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:116
		{
		}
	case 8:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:126
		{
			loggo.Debug("[yacc]: include %v", yyDollar[2].s)
			l := yylex.(lexerwarpper).mf
			l.add_include(yyDollar[2].s)
		}
	case 9:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:135
		{
		}
	case 12:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:145
		{
			loggo.Debug("[yacc]: struct_define %v", yyDollar[2].s)
			l := yylex.(lexerwarpper).mf
			if (yyDollar[3].sn) != nil {
				p := (yyDollar[3].sn).(*struct_desc_memlist_node)
				l.add_struct_desc(yyDollar[2].s, p)
			}
		}
	case 13:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:157
		{
			yyVAL.sn = nil
		}
	case 14:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:162
		{
			loggo.Debug("[yacc]: struct_mem_declaration <- IDENTIFIER struct_mem_declaration")
			p := (yyDollar[1].sn).(*struct_desc_memlist_node)
			p.add_arg(yyDollar[2].s)
			yyVAL.sn = p
		}
	case 15:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:170
		{
			loggo.Debug("[yacc]: struct_mem_declaration <- IDENTIFIER")
			p := &struct_desc_memlist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_arg(yyDollar[1].s)
			yyVAL.sn = p
		}
	case 16:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:180
		{
		}
	case 19:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:190
		{
			loggo.Debug("[yacc]: const_define %v", yyDollar[2].s)
			l := yylex.(lexerwarpper).mf
			l.add_const_desc(yyDollar[2].s, yyDollar[4].sn)
		}
	case 20:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:199
		{
		}
	case 23:
		yyDollar = yyS[yypt-7 : yypt+1]
		//line ../yacc.y:211
		{
			loggo.Debug("[yacc]: function_declaration <- block %v", yyDollar[2].s)
			p := &func_desc_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.funcname = yyDollar[2].s
			p.arglist = yyDollar[4].sn.(*func_desc_arglist_node)
			p.block = yyDollar[6].sn.(*block_node)
			l := yylex.(lexerwarpper).mf
			l.add_func_desc(p)
		}
	case 24:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line ../yacc.y:222
		{
			loggo.Debug("[yacc]: function_declaration <- empty %v", yyDollar[2].s)
			p := &func_desc_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.funcname = yyDollar[2].s
			p.arglist = yyDollar[4].sn.(*func_desc_arglist_node)
			p.block = nil
			l := yylex.(lexerwarpper).mf
			l.add_func_desc(p)
		}
	case 25:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:235
		{
			yyVAL.sn = nil
		}
	case 26:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:240
		{
			loggo.Debug("[yacc]: function_declaration_arguments <- arg function_declaration_arguments")
			p := (yyDollar[1].sn).(*func_desc_arglist_node)
			p.add_arg(yyDollar[3].s)
			yyVAL.sn = p
		}
	case 27:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:248
		{
			loggo.Debug("[yacc]: function_declaration_arguments <- arg")
			p := &func_desc_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_arg(yyDollar[1].s)
			yyVAL.sn = p
		}
	case 28:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:258
		{
			loggo.Debug("[yacc]: arg <- IDENTIFIER %v", yyDollar[1].s)
			p := &identifier_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.str = yyDollar[1].s
			yyVAL.sn = p
		}
	case 29:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:268
		{
			loggo.Debug("[yacc]: function_call <- function_call_arguments %v", yyDollar[1].s)
			p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.fuc = yyDollar[1].s
			p.prefunc = nil
			p.arglist = (yyDollar[3].sn).(*function_call_arglist_node)
			p.fakecall = false
			p.classmem_call = false
			yyVAL.sn = p
		}
	case 30:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:280
		{
			loggo.Debug("[yacc]: function_call <- function_call_arguments %v", yyDollar[1].s)
			p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.fuc = yyDollar[1].s
			p.prefunc = nil
			p.arglist = (yyDollar[3].sn).(*function_call_arglist_node)
			p.fakecall = false
			p.classmem_call = false
			yyVAL.sn = p
		}
	case 31:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:292
		{
			loggo.Debug("[yacc]: function_call <- function_call_arguments")
			p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.fuc = ""
			p.prefunc = yyDollar[1].sn
			p.arglist = (yyDollar[3].sn).(*function_call_arglist_node)
			p.fakecall = false
			p.classmem_call = false
			yyVAL.sn = p
		}
	case 32:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line ../yacc.y:304
		{
			loggo.Debug("[yacc]: function_call <- mem function_call_arguments %v", yyDollar[3].s)
			p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.fuc = yyDollar[3].s
			p.prefunc = nil
			if yyDollar[5].sn == nil {
				p.arglist = &function_call_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			} else {
				p.arglist = (yyDollar[5].sn).(*function_call_arglist_node)
			}
			p.arglist.add_arg(yyDollar[1].sn)
			p.fakecall = false
			p.classmem_call = true
			yyVAL.sn = p
		}
	case 33:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line ../yacc.y:321
		{
			loggo.Debug("[yacc]: function_call <- mem function_call_arguments %v", yyDollar[3].s)
			p := &function_call_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.fuc = yyDollar[3].s
			p.prefunc = nil
			if yyDollar[5].sn == nil {
				p.arglist = &function_call_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			} else {
				p.arglist = (yyDollar[5].sn).(*function_call_arglist_node)
			}
			p.arglist.add_arg(yyDollar[1].sn)
			p.fakecall = false
			p.classmem_call = true
			yyVAL.sn = p
		}
	case 34:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:340
		{
			yyVAL.sn = nil
		}
	case 35:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:345
		{
			loggo.Debug("[yacc]: function_call_arguments <- arg_expr function_call_arguments")
			p := (yyDollar[1].sn).(*function_call_arglist_node)
			p.add_arg(yyDollar[3].sn)
			yyVAL.sn = p
		}
	case 36:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:353
		{
			loggo.Debug("[yacc]: function_call_arguments <- arg_expr")
			p := &function_call_arglist_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_arg(yyDollar[1].sn)
			yyVAL.sn = p
		}
	case 37:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:363
		{
			loggo.Debug("[yacc]: arg_expr <- expr_value")
			yyVAL.sn = yyDollar[1].sn
		}
	case 38:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:373
		{
			loggo.Debug("[yacc]: block <- block stmt")
			p := (yyDollar[1].sn).(*block_node)
			p.add_stmt(yyDollar[2].sn)
			yyVAL.sn = p
		}
	case 39:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:381
		{
			loggo.Debug("[yacc]: block <- stmt")
			p := &block_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_stmt(yyDollar[1].sn)
			yyVAL.sn = p
		}
	case 40:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:391
		{
			loggo.Debug("[yacc]: stmt <- while_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 41:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:397
		{
			loggo.Debug("[yacc]: stmt <- if_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 42:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:403
		{
			loggo.Debug("[yacc]: stmt <- return_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 43:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:409
		{
			loggo.Debug("[yacc]: stmt <- assign_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 44:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:415
		{
			loggo.Debug("[yacc]: stmt <- multi_assign_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 45:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:421
		{
			loggo.Debug("[yacc]: stmt <- break")
			yyVAL.sn = yyDollar[1].sn
		}
	case 46:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:427
		{
			loggo.Debug("[yacc]: stmt <- continue")
			yyVAL.sn = yyDollar[1].sn
		}
	case 47:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:433
		{
			loggo.Debug("[yacc]: stmt <- expr")
			yyVAL.sn = yyDollar[1].sn
		}
	case 48:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:439
		{
			loggo.Debug("[yacc]: stmt <- math_assign_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 49:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:445
		{
			loggo.Debug("[yacc]: stmt <- for_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 50:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:451
		{
			loggo.Debug("[yacc]: stmt <- for_loop_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 51:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:457
		{
			loggo.Debug("[yacc]: stmt <- fake_call_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 52:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:463
		{
			loggo.Debug("[yacc]: stmt <- sleep_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 53:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:469
		{
			loggo.Debug("[yacc]: stmt <- yield_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 54:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:475
		{
			loggo.Debug("[yacc]: stmt <- switch_stmt")
			yyVAL.sn = yyDollar[1].sn
		}
	case 55:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:483
		{
			loggo.Debug("[yacc]: fake_call_stmt <- fake function_call")
			p := (yyDollar[2].sn).(*function_call_node)
			p.fakecall = true
			yyVAL.sn = p
		}
	case 56:
		yyDollar = yyS[yypt-9 : yypt+1]
		//line ../yacc.y:493
		{
			loggo.Debug("[yacc]: for_stmt <- block cmp block")
			p := &for_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[4].sn).(*cmp_stmt)
			p.beginblock = (yyDollar[2].sn).(*block_node)
			p.endblock = (yyDollar[6].sn).(*block_node)
			p.block = (yyDollar[8].sn).(*block_node)
			yyVAL.sn = p
		}
	case 57:
		yyDollar = yyS[yypt-8 : yypt+1]
		//line ../yacc.y:504
		{
			loggo.Debug("[yacc]: for_stmt <- block cmp")
			p := &for_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[4].sn).(*cmp_stmt)
			p.beginblock = (yyDollar[2].sn).(*block_node)
			p.endblock = (yyDollar[6].sn).(*block_node)
			p.block = nil
			yyVAL.sn = p
		}
	case 58:
		yyDollar = yyS[yypt-11 : yypt+1]
		//line ../yacc.y:517
		{
			loggo.Debug("[yacc]: for_loop_stmt <- block")
			p := &for_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}

			pi := yyDollar[2].sn
			if pi.gettype() == est_var {
				pvar := &variable_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
				pvar.str = (pi).(*var_node).str
				pi = pvar
			}

			pcmp := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pcmp.cmp = "<"
			pcmp.left = pi
			pcmp.right = yyDollar[6].sn
			p.cmp = pcmp

			pbeginblockassign := &assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pbeginblockassign.vr = yyDollar[2].sn
			pbeginblockassign.value = yyDollar[4].sn
			pbeginblockassign.isnew = false
			pbeginblock := &block_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pbeginblock.add_stmt(pbeginblockassign)
			p.beginblock = pbeginblock

			pendblockassign := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pendblockassign.vr = pi
			pendblockassign.oper = "+="
			pendblockassign.value = yyDollar[8].sn
			pendblock := &block_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pendblock.add_stmt(pendblockassign)
			p.endblock = pendblock

			p.block = (yyDollar[10].sn).(*block_node)
			yyVAL.sn = p
		}
	case 59:
		yyDollar = yyS[yypt-10 : yypt+1]
		//line ../yacc.y:555
		{
			loggo.Debug("[yacc]: for_loop_stmt <- empty")
			p := &for_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}

			pcmp := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pcmp.cmp = "<"
			pcmp.left = yyDollar[2].sn
			pcmp.right = yyDollar[6].sn
			p.cmp = pcmp

			pbeginblockassign := &assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pbeginblockassign.vr = yyDollar[2].sn
			pbeginblockassign.value = yyDollar[4].sn
			pbeginblockassign.isnew = false
			pbeginblock := &block_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pbeginblock.add_stmt(pbeginblockassign)
			p.beginblock = pbeginblock

			pendblockassign := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pendblockassign.vr = yyDollar[2].sn
			pendblockassign.oper = "+="
			pendblockassign.value = yyDollar[8].sn
			pendblock := &block_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pendblock.add_stmt(pendblockassign)
			p.endblock = pendblock

			p.block = nil
			yyVAL.sn = p
		}
	case 60:
		yyDollar = yyS[yypt-5 : yypt+1]
		//line ../yacc.y:588
		{
			loggo.Debug("[yacc]: while_stmt <- cmp block")
			p := &while_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = (yyDollar[4].sn).(*block_node)
			yyVAL.sn = p
		}
	case 61:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:597
		{
			loggo.Debug("[yacc]: while_stmt <- cmp")
			p := &while_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = nil
			yyVAL.sn = p
		}
	case 62:
		yyDollar = yyS[yypt-7 : yypt+1]
		//line ../yacc.y:608
		{
			loggo.Debug("[yacc]: if_stmt <- cmp block")
			p := &if_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = (yyDollar[4].sn).(*block_node)
			p.elseifs = (yyDollar[5].sn).(*elseif_stmt_list)
			p.elses = (yyDollar[6].sn).(*else_stmt)
			yyVAL.sn = p
		}
	case 63:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line ../yacc.y:619
		{
			loggo.Debug("[yacc]: if_stmt <- cmp")
			p := &if_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = nil
			p.elseifs = (yyDollar[4].sn).(*elseif_stmt_list)
			p.elses = (yyDollar[5].sn).(*else_stmt)
			yyVAL.sn = p
		}
	case 64:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:632
		{
			yyVAL.sn = nil
		}
	case 65:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:637
		{
			loggo.Debug("[yacc]: elseif_stmt_list <- elseif_stmt_list elseif_stmt")
			p := (yyDollar[1].sn).(*elseif_stmt_list)
			p.add_stmt(yyDollar[2].sn)
			yyVAL.sn = p
		}
	case 66:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:645
		{
			loggo.Debug("[yacc]: elseif_stmt_list <- elseif_stmt")
			p := &elseif_stmt_list{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_stmt(yyDollar[1].sn)
			yyVAL.sn = p
		}
	case 67:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:655
		{
			loggo.Debug("[yacc]: elseif_stmt <- ELSEIF cmp THEN block")
			p := &elseif_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = (yyDollar[4].sn).(*block_node)
			yyVAL.sn = p
		}
	case 68:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:664
		{
			loggo.Debug("[yacc]: elseif_stmt <- ELSEIF cmp THEN block")
			p := &elseif_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = (yyDollar[2].sn).(*cmp_stmt)
			p.block = nil
			yyVAL.sn = p
		}
	case 69:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:675
		{
			yyVAL.sn = nil
		}
	case 70:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:680
		{
			loggo.Debug("[yacc]: else_stmt <- block")
			p := &else_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.block = (yyDollar[2].sn).(*block_node)
			yyVAL.sn = p
		}
	case 71:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:688
		{
			loggo.Debug("[yacc]: else_stmt <- empty")
			p := &else_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.block = nil
			yyVAL.sn = p
		}
	case 72:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:698
		{
			loggo.Debug("[yacc]: cmp <- ( cmp )")
			yyVAL.sn = yyDollar[2].sn
		}
	case 73:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:704
		{
			loggo.Debug("[yacc]: cmp <- cmp AND cmp")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "&&"
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 74:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:714
		{
			loggo.Debug("[yacc]: cmp <- cmp OR cmp")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "||"
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 75:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:724
		{
			loggo.Debug("[yacc]: cmp <- cmp_value LESS cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 76:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:734
		{
			loggo.Debug("[yacc]: cmp <- cmp_value MORE cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 77:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:744
		{
			loggo.Debug("[yacc]: cmp <- cmp_value EQUAL cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 78:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:754
		{
			loggo.Debug("[yacc]: cmp <- cmp_value MORE_OR_EQUAL cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 79:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:764
		{
			loggo.Debug("[yacc]: cmp <- cmp_value LESS_OR_EQUAL cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 80:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:774
		{
			loggo.Debug("[yacc]: cmp <- cmp_value NOT_EQUAL cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = yyDollar[2].s
			p.left = yyDollar[1].sn
			p.right = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 81:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:784
		{
			loggo.Debug("[yacc]: cmp <- true")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "true"
			p.left = nil
			p.right = nil
			yyVAL.sn = p
		}
	case 82:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:794
		{
			loggo.Debug("[yacc]: cmp <- false")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "false"
			p.left = nil
			p.right = nil
			yyVAL.sn = p
		}
	case 83:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:804
		{
			loggo.Debug("[yacc]: cmp <- cmp_value IS cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "is"
			p.left = yyDollar[2].sn
			p.right = nil
			yyVAL.sn = p
		}
	case 84:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:814
		{
			loggo.Debug("[yacc]: cmp <- cmp_value NOT cmp_value")
			p := &cmp_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.cmp = "not"
			p.left = yyDollar[2].sn
			p.right = nil
			yyVAL.sn = p
		}
	case 85:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:826
		{
			loggo.Debug("[yacc]: cmp_value <- explicit_value")
			yyVAL.sn = yyDollar[1].sn
		}
	case 86:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:832
		{
			loggo.Debug("[yacc]: cmp_value <- variable")
			yyVAL.sn = yyDollar[1].sn
		}
	case 87:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:838
		{
			loggo.Debug("[yacc]: cmp_value <- expr")
			yyVAL.sn = yyDollar[1].sn
		}
	case 88:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:846
		{
			loggo.Debug("[yacc]: return_stmt <- RETURN return_value_list")
			p := &return_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.returnlist = (yyDollar[2].sn).(*return_value_list_node)
			yyVAL.sn = p
		}
	case 89:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:854
		{
			loggo.Debug("[yacc]: return_stmt <- RETURN")
			p := &return_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.returnlist = nil
			yyVAL.sn = p
		}
	case 90:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:864
		{
			loggo.Debug("[yacc]: return_value_list <- return_value_list return_value")
			p := (yyDollar[1].sn).(*return_value_list_node)
			p.add_arg(yyDollar[3].sn)
			yyVAL.sn = p
		}
	case 91:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:872
		{
			loggo.Debug("[yacc]: return_value_list <- return_value")
			p := &return_value_list_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_arg(yyDollar[1].sn)
			yyVAL.sn = p
		}
	case 92:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:882
		{
			loggo.Debug("[yacc]: return_value <- explicit_value")
			yyVAL.sn = yyDollar[1].sn
		}
	case 93:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:888
		{
			loggo.Debug("[yacc]: return_value <- variable")
			yyVAL.sn = yyDollar[1].sn
		}
	case 94:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:894
		{
			loggo.Debug("[yacc]: return_value <- expr")
			yyVAL.sn = yyDollar[1].sn
		}
	case 95:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:902
		{
			loggo.Debug("[yacc]: assign_stmt <- var assign_value")
			p := &assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.value = yyDollar[3].sn
			p.isnew = false
			yyVAL.sn = p
		}
	case 96:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:912
		{
			loggo.Debug("[yacc]: new assign_stmt <- var assign_value")
			p := &assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.value = yyDollar[3].sn
			p.isnew = true
			yyVAL.sn = p
		}
	case 97:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:924
		{
			loggo.Debug("[yacc]: multi_assign_stmt <- var_list function_call")
			p := &multi_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.varlist = (yyDollar[1].sn).(*var_list_node)
			p.value = yyDollar[3].sn
			p.isnew = false
			yyVAL.sn = p
		}
	case 98:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:934
		{
			loggo.Debug("[yacc]: new multi_assign_stmt <- var_list function_call")
			p := &multi_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.varlist = (yyDollar[1].sn).(*var_list_node)
			p.value = yyDollar[3].sn
			p.isnew = true
			yyVAL.sn = p
		}
	case 99:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:946
		{
			loggo.Debug("[yacc]: var_list <- var_list var")
			p := (yyDollar[1].sn).(*var_list_node)
			p.add_arg(yyDollar[3].sn)
			yyVAL.sn = p
		}
	case 100:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:954
		{
			loggo.Debug("[yacc]: var_list <- var")
			p := &var_list_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.add_arg(yyDollar[1].sn)
			yyVAL.sn = p
		}
	case 101:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:964
		{
			loggo.Debug("[yacc]: assign_value <- explicit_value")
			yyVAL.sn = yyDollar[1].sn
		}
	case 102:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:970
		{
			loggo.Debug("[yacc]: assign_value <- variable")
			yyVAL.sn = yyDollar[1].sn
		}
	case 103:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:976
		{
			loggo.Debug("[yacc]: assign_value <- expr")
			yyVAL.sn = yyDollar[1].sn
		}
	case 104:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:984
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value")
			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "+="
			p.value = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 105:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:994
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value")
			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "-="
			p.value = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 106:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1004
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value")
			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "/="
			p.value = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 107:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1014
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value")
			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "*="
			p.value = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 108:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1024
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable assign_value")
			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "%="
			p.value = yyDollar[3].sn
			yyVAL.sn = p
		}
	case 109:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1034
		{
			loggo.Debug("[yacc]: math_assign_stmt <- variable INC")
			pp := &explicit_value_node{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			pp.str = "1"
			pp.ty = EVT_NUM

			p := &math_assign_stmt{syntree_node_base: syntree_node_base{yylex.(lexerwarpper).yyLexer.(*Lexer).Line()}}
			p.vr = yyDollar[1].sn
			p.oper = "+="
			p.value = pp
			yyVAL.sn = p
		}
	case 110:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1050
		{
			loggo.Debug("[yacc]: var <- VAR_BEGIN IDENTIFIER %v", yyDollar[2].s)
			//NEWTYPE(p, var_node);
			//p->str = $2;
			//$$ = p;
		}
	case 111:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1058
		{
			loggo.Debug("[yacc]: var <- variable")
			//$$ = $1;
		}
	case 112:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1066
		{
			loggo.Debug("[yacc]: variable <- IDENTIFIER %v", yyDollar[1].s)
			//NEWTYPE(p, variable_node);
			//p->str = $1;
			//$$ = p;
		}
	case 113:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:1074
		{
			loggo.Debug("[yacc]: container_get_node <- IDENTIFIER[expr_value] %v", yyDollar[1].s)
			//NEWTYPE(p, container_get_node);
			//p->container = $1;
			//p->key = $3;
			//$$ = p;
		}
	case 114:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1083
		{
			loggo.Debug("[yacc]: variable <- IDENTIFIER_POINTER %v", yyDollar[1].s)
			//NEWTYPE(p, struct_pointer_node);
			//p->str = $1;
			//$$ = p;
		}
	case 115:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1091
		{
			loggo.Debug("[yacc]: variable <- IDENTIFIER_DOT %v", yyDollar[1].s)
			//NEWTYPE(p, variable_node);
			//p->str = $1;
			//$$ = p;
		}
	case 116:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1101
		{
			loggo.Debug("[yacc]: expr <- (expr)")
			//$$ = $2;
		}
	case 117:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1107
		{
			loggo.Debug("[yacc]: expr <- function_call")
			//$$ = $1;
		}
	case 118:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1113
		{
			loggo.Debug("[yacc]: expr <- math_expr")
			//$$ = $1;
		}
	case 119:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1121
		{
			loggo.Debug("[yacc]: math_expr <- (math_expr)")
			//$$ = $2;
		}
	case 120:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1127
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "+";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 121:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1137
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "-";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 122:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1147
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "*";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 123:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1157
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "/";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 124:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1167
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "%";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 125:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1177
		{
			loggo.Debug("[yacc]: math_expr <- expr_value %v expr_value", yyDollar[2].s)
			//NEWTYPE(p, math_expr_node);
			//p->oper = "..";
			//p->left = $1;
			//p->right = $3;
			//$$ = p;
		}
	case 126:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1189
		{
			loggo.Debug("[yacc]: expr_value <- math_expr")
			//$$ = $1;
		}
	case 127:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1195
		{
			loggo.Debug("[yacc]: expr_value <- explicit_value")
			//$$ = $1;
		}
	case 128:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1201
		{
			loggo.Debug("[yacc]: expr_value <- function_call")
			//$$ = $1;
		}
	case 129:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1207
		{
			loggo.Debug("[yacc]: expr_value <- variable")
			//$$ = $1;
		}
	case 130:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1215
		{
			loggo.Debug("[yacc]: explicit_value <- FTRUE")
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_TRUE;
			//$$ = p;
		}
	case 131:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1224
		{
			loggo.Debug("[yacc]: explicit_value <- FFALSE")
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_FALSE;
			//$$ = p;
		}
	case 132:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1233
		{
			loggo.Debug("[yacc]: explicit_value <- NUMBER %v", yyDollar[1].s)
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_NUM;
			//$$ = p;
		}
	case 133:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1242
		{
			loggo.Debug("[yacc]: explicit_value <- FKUUID %v", yyDollar[1].s)
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_UUID;
			//$$ = p;
		}
	case 134:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1251
		{
			loggo.Debug("[yacc]: explicit_value <- STRING_DEFINITION %v", yyDollar[1].s)
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_STR;
			//$$ = p;
		}
	case 135:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1260
		{
			loggo.Debug("[yacc]: explicit_value <- FKFLOAT %v", yyDollar[1].s)
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_FLOAT;
			//$$ = p;
		}
	case 136:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1269
		{
			loggo.Debug("[yacc]: explicit_value <- FNULL %v", yyDollar[1].s)
			//NEWTYPE(p, explicit_value_node);
			//p->str = $1;
			//p->type = explicit_value_node::EVT_NULL;
			//$$ = p;
		}
	case 137:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1278
		{
			loggo.Debug("[yacc]: explicit_value <- const_map_list_value")
			//NEWTYPE(p, explicit_value_node);
			//p->str = "";
			//p->type = explicit_value_node::EVT_MAP;
			//p->v = $2;
			//$$ = p;
		}
	case 138:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1288
		{
			loggo.Debug("[yacc]: explicit_value <- const_array_list_value")
			//NEWTYPE(p, explicit_value_node);
			//p->str = "";
			//p->type = explicit_value_node::EVT_ARRAY;
			//p->v = $2;
			//$$ = p;
		}
	case 139:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:1300
		{
			loggo.Debug("[yacc]: const_map_list_value <- null")
			//NEWTYPE(p, const_map_list_value_node);
			//$$ = p;
		}
	case 140:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1307
		{
			loggo.Debug("[yacc]: const_map_list_value <- const_map_value")
			//NEWTYPE(p, const_map_list_value_node);
			//p->add_ele($1);
			//$$ = p;
		}
	case 141:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1315
		{
			loggo.Debug("[yacc]: const_map_list_value <- const_map_list_value const_map_value")
			//assert($1->gettype() == est_constmaplist);
			//const_map_list_value_node * p = dynamic_cast<const_map_list_value_node*>($1);
			//p->add_ele($2);
			//$$ = p;
		}
	case 142:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1326
		{
			loggo.Debug("[yacc]: const_map_value <- explicit_value")
			//NEWTYPE(p, const_map_value_node);
			//p->k = $1;
			//p->v = $3;
			//$$ = p;
		}
	case 143:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line ../yacc.y:1337
		{
			loggo.Debug("[yacc]: const_array_list_value <- null")
			//NEWTYPE(p, const_array_list_value_node);
			//$$ = p;
		}
	case 144:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1344
		{
			loggo.Debug("[yacc]: const_array_list_value <- explicit_value")
			//NEWTYPE(p, const_array_list_value_node);
			//p->add_ele($1);
			//$$ = p;
		}
	case 145:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1352
		{
			loggo.Debug("[yacc]: const_array_list_value <- const_array_list_value explicit_value")
			//assert($1->gettype() == est_constarraylist);
			//const_array_list_value_node * p = dynamic_cast<const_array_list_value_node*>($1);
			//p->add_ele($2);
			//$$ = p;
		}
	case 146:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1363
		{
			loggo.Debug("[yacc]: break <- BREAK")
			//NEWTYPE(p, break_stmt);
			//$$ = p;
		}
	case 147:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1372
		{
			loggo.Debug("[yacc]: CONTINUE")
			//NEWTYPE(p, continue_stmt);
			//$$ = p;
		}
	case 148:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1381
		{
			loggo.Debug("[yacc]: SLEEP")
			//NEWTYPE(p, sleep_stmt);
			//p->time = $2;
			//$$ = p;
		}
	case 149:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1390
		{
			loggo.Debug("[yacc]: YIELD")
			//NEWTYPE(p, yield_stmt);
			//p->time = $2;
			//$$ = p;
		}
	case 150:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line ../yacc.y:1400
		{
			loggo.Debug("[yacc]: switch_stmt")
			//NEWTYPE(p, switch_stmt);
			//p->cmp = $2;
			//p->caselist = $3;
			//p->def = $5;
			//$$ = p;
		}
	case 151:
		yyDollar = yyS[yypt-5 : yypt+1]
		//line ../yacc.y:1410
		{
			loggo.Debug("[yacc]: switch_stmt")
			//NEWTYPE(p, switch_stmt);
			//p->cmp = $2;
			//p->caselist = $3;
			//p->def = 0;
			//$$ = p;
		}
	case 152:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line ../yacc.y:1422
		{
			loggo.Debug("[yacc]: switch_case_list <- switch_case_define")
			//NEWTYPE(p, switch_caselist_node);
			//p->add_case($1);
			//$$ = p;
		}
	case 153:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line ../yacc.y:1430
		{
			loggo.Debug("[yacc]: switch_case_list <- switch_case_list switch_case_define")
			//assert($2->gettype() == est_switch_case_node);
			//switch_caselist_node * p = dynamic_cast<switch_caselist_node*>($1);
			//p->add_case($2);
			//$$ = p;
		}
	case 154:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line ../yacc.y:1441
		{
			loggo.Debug("[yacc]: switch_case_define")
			//NEWTYPE(p, switch_case_node);
			//p->cmp = $2;
			//p->block = $4;
			//$$ = p;
		}
	case 155:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line ../yacc.y:1450
		{
			loggo.Debug("[yacc]: switch_case_define")
			//NEWTYPE(p, switch_case_node);
			//p->cmp = $2;
			//p->block = 0;
			//$$ = p;
		}
	}
	goto yystack /* stack new state and value */
}
