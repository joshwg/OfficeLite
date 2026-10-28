package mathtrig

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// regData: the office-building example of Microsoft's LINEST page in
// A1:E11 (floor space, offices, entrances, age, assessed value); G1:G6 =
// 7, 3.5, 3.5, 1, 2, "x" (text); H1:H6 = 1, 2, "a", 4, TRUE, 6.
func regData(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{
		{2310, 2, 2, 20, 142000}, {2333, 2, 2, 12, 144000}, {2356, 3, 1.5, 33, 151000},
		{2379, 3, 2, 43, 150000}, {2402, 2, 3, 53, 139000}, {2425, 4, 2, 23, 169000},
		{2448, 2, 1.5, 99, 126000}, {2471, 2, 2, 34, 142900}, {2494, 3, 3, 23, 163000},
		{2517, 4, 4, 55, 169000}, {2540, 2, 3, 22, 149000},
	})
	ctx.SetRange("G1", [][]any{{7, 1}, {3.5, 2}, {3.5, "a"}, {1, 4}, {2, true}, {"x", 6}})
}

func TestDescriptive(t *testing.T) {
	ftest.Run(t, registry(), regData, []ftest.Case{
		{"=GEOMEAN(4,5,8,7,11,4,3)", near(5.476986969)},
		{"=GEOMEAN(4,0)", f.ErrNum},
		{"=GEOMEAN(G6)", f.ErrNum},
		{"=HARMEAN(4,5,8,7,11,4,3)", near(5.028375962)},
		{"=HARMEAN(-1,2)", f.ErrNum},
		{"=AVEDEV(4,5,6,7,5,4,3)", near(1.020408163)},
		{"=AVEDEV(G6)", f.ErrNum},
		{"=DEVSQ(4,5,8,7,11,4,3)", 48},
		{"=KURT(3,4,5,2,3,4,5,6,4,7)", near(-0.151799637)},
		{"=KURT(1,2,3)", f.ErrDiv0},
		{"=KURT(1,1,1,1)", f.ErrDiv0},
		{"=SKEW(3,4,5,2,3,4,5,6,4,7)", near(0.359543)},
		{"=SKEW(1,2)", f.ErrDiv0},
		{"=SKEW.P(3,4,5,2,3,4,5,6,4,7)", near(0.303193)},
		{"=SKEW.P(5)", f.ErrDiv0},
		{"=TRIMMEAN({4,5,6,7,2,3,4,5,1,2,3},0.2)", near(3.777777778)},
		{"=TRIMMEAN({1,2,3,100},0.5)", 2.5},
		{"=TRIMMEAN({1,2,3},1)", f.ErrNum},
		{"=TRIMMEAN({1,2,3},-0.1)", f.ErrNum},
		{"=STANDARDIZE(42,40,1.5)", near(1.333333333)},
		{"=STANDARDIZE(42,40,0)", f.ErrNum},
		// RANK ignores the text in G6.
		{"=RANK(G2,G1:G6,1)", 3},
		{"=RANK(G1,G1:G6,1)", 5},
		{"=RANK(7,G1:G6)", 1},
		{"=RANK.EQ(3.5,G1:G6)", 2},
		{"=RANK.AVG(3.5,G1:G6)", 2.5},
		{"=RANK.AVG(94,{89,88,92,101,94,97,95})", 4},
		{"=RANK(5,G1:G6)", f.ErrNA},
		{"=RANK(G1:G2,G1:G6)", ftest.Arr{{1}, {2}}},
		{"=PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},2)", 0.333},
		{"=PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},4)", 0.555},
		{"=PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},8)", 0.666},
		{"=PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},5)", 0.583},
		{"=PERCENTRANK({13,12,11,8,4,3,2,1,1,1},1)", 0},
		{"=PERCENTRANK({13,12,11,8,4,3,2,1,1,1},13)", 1},
		{"=PERCENTRANK({13,12,11,8,4,3,2,1,1,1},5,5)", 0.58333},
		{"=PERCENTRANK({13,12,11,8,4,3,2,1,1,1},14)", f.ErrNA},
		{"=PERCENTRANK({1,2},1,0)", f.ErrNum},
		{"=PERCENTRANK.EXC({1,2,3,6,6,6,7,8,9},7)", 0.7},
		{"=PERCENTRANK.EXC({1,2,3,6,6,6,7,8,9},5.43)", 0.381},
		{"=PERCENTRANK.EXC({1,2,3,6,6,6,7,8,9},5.43,1)", 0.3},
		{"=FREQUENCY({79,85,78,85,50,81,95,88,97},{70,79,89})", ftest.Arr{{1}, {2}, {4}, {2}}},
		{"=FREQUENCY({79,85,78,85,50,81,95,88,97},{89,70,79})", ftest.Arr{{4}, {1}, {2}, {2}}},
		{"=FREQUENCY(G1:G6,H1:H6)", ftest.Arr{{1}, {1}, {2}, {0}, {1}}},
		{"=FREQUENCY({1,2,3},G6)", 3}, // no bins: one count
		{"=PERMUT(100,3)", 970200},
		{"=PERMUT(3,2)", 6},
		{"=PERMUT(2,3)", f.ErrNum},
		{"=PERMUTATIONA(3,2)", 9},
		{"=PERMUTATIONA(2,2)", 4},
		{"=PERMUTATIONA(-1,2)", f.ErrNum},
		{"=FISHER(0.75)", near(0.972955075)},
		{"=FISHER(1)", f.ErrNum},
		{"=FISHERINV(0.972955075)", near(0.75)},
	})
}

func TestPaired(t *testing.T) {
	ftest.Run(t, registry(), regData, []ftest.Case{
		{"=CORREL({3,2,4,5,6},{9,7,12,15,17})", near(0.997054486)},
		{"=PEARSON({9,7,5,3,1},{10,6,1,5,3})", near(0.699379)},
		{"=RSQ({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", near(0.05795)},
		{"=SLOPE({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", near(0.305556)},
		{"=INTERCEPT({2,3,9,1,8},{6,5,11,7,5})", near(0.0483871)},
		{"=STEYX({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})", near(3.305719)},
		{"=COVAR({3,2,4,5,6},{9,7,12,15,17})", 5.2},
		{"=COVARIANCE.P({3,2,4,5,6},{9,7,12,15,17})", 5.2},
		{"=COVARIANCE.S({2,4,8},{5,11,12})", near(9.666666667)},
		{"=FORECAST(30,{6,7,9,15,21},{20,28,31,38,40})", near(10.607253)},
		{"=FORECAST.LINEAR(30,{6,7,9,15,21},{20,28,31,38,40})", near(10.607253)},
		{"=FORECAST({30,40},{6,7,9,15,21},{20,28,31,38,40})", ftest.Arr{{near(10.607253), near(17.698302469)}}},
		// Pairs with a non-number on either side are skipped: (7,1),
		// (3.5,2), (1,4) remain.
		{"=SLOPE(G1:G6,H1:H6)", near(-1.892857143)},
		{"=COVARIANCE.S(G1:G6,H1:H6)", near(-4.416666667)},
		// A row against a column of the same size pairs element by element.
		{"=CORREL({1,2,3},{2;4;7})", near(0.9933992678)},
		{"=CORREL({1,2,3},{1,2})", f.ErrNA},
		{"=CORREL({1,1,1},{1,2,3})", f.ErrDiv0},
		{"=CORREL(G6,H6)", f.ErrDiv0},
		{"=SLOPE({1,2},{3,3})", f.ErrDiv0},
		{"=STEYX({1,2},{3,4})", f.ErrDiv0},
		{"=COVARIANCE.S({1},{2})", f.ErrDiv0},
		{"=CORREL({1,#N/A},{1,2})", f.ErrNA},
	})
}

func TestLinest(t *testing.T) {
	ftest.Run(t, registry(), regData, []ftest.Case{
		{"=LINEST({1,9,5,7},{0,4,2,3})", ftest.Arr{{2, 1}}},
		{"=LINEST({3100,4500,4400,5400,7500,8100})", ftest.Arr{{1000, 2000}}},
		{"=LINEST({3100;4500;4400;5400;7500;8100},,FALSE)", ftest.Arr{{near(1461.538462), 0}}},
		{"=LINEST({3100;4500;4400;5400;7500;8100},,TRUE,TRUE)", ftest.Arr{
			{1000, 2000},
			{133.0950251297385, 518.3306537980045},
			{0.9338313767342582, 556.7764362830022},
			{56.45161290322576, 4},
			{17499999.999999985, 1240000.0},
		}},
		{"=LINEST(E1:E11,A1:D11,TRUE,TRUE)", ftest.Arr{
			{-234.2371644712057, 2553.210660391569, 12529.768167086739, 27.641387366020155, 52317.830507291816},
			{13.268011475500254, 530.6691519303666, 400.06683819395016, 5.429374041545004, 12237.36160286162},
			{0.9967479933845101, 970.5784629284992, f.ErrNA, f.ErrNA, f.ErrNA},
			{459.7536742254014, 6, f.ErrNA, f.ErrNA, f.ErrNA},
			{1732393319.2292595, 5652135.316203889, f.ErrNA, f.ErrNA, f.ErrNA},
		}},
		{"=LINEST(E1:E11,A1:D11,FALSE,TRUE)", ftest.Arr{
			{-250.13503238418343, 1248.79522496476, 12540.261860152552, 50.71193750640334, 0},
			{23.719062469351513, 808.5983028790091, 745.0407107904955, 1.114377349765304, f.ErrNA},
			{0.9999076735088338, 1807.5331643135212, f.ErrNA, f.ErrNA, f.ErrNA},
			{18952.72317334794, 7, f.ErrNA, f.ErrNA, f.ErrNA},
			{247687539767.01935, 22870232.980652757, f.ErrNA, f.ErrNA, f.ErrNA},
		}},
		// A variable collinear with another gets a zero coefficient.
		{"=LINEST({3;5;7;9;12},{1,2;2,4;3,6;4,8;5,10})", ftest.Arr{{0, 2.2, near(0.6)}}},
		{"=LINEST({1,2,3},{1,2})", f.ErrRef},
		{"=LINEST({1,\"a\",3})", f.ErrValue},
		{"=LOGEST({33100,47300,69000,102000,150000,220000},{11,12,13,14,15,16})",
			ftest.Arr{{1.4632756281, 495.3047701611}}},
		{"=LOGEST({1,-2,3})", f.ErrNum},
		// LOGEST's statistics are those of the linear fit of ln y.
		{"=LOGEST({33100;47300;69000;102000;150000;220000},{11;12;13;14;15;16},TRUE,TRUE)", ftest.Arr{
			{1.4632756281, 495.3047701611},
			{0.0026334028914250527, 0.035834282435717506},
			{0.9998086197758176, 0.011016314665073008},
			{20896.80109942032, 4},
			{2.5360188299385635, 0.00048543675519961053},
		}},
		{"=TREND({133890,135000,135790,137300,138130,139100,139900,141120,141890,143230,144000,145290},{1,2,3,4,5,6,7,8,9,10,11,12},{13,14,15,16,17})",
			ftest.Arr{{146171.51515151517, 147189.696969697, 148207.8787878788, 149226.06060606064, 150244.24242424246}}},
		{"=TREND({1;9;5;7},{0;4;2;3})", ftest.Arr{{1}, {9}, {5}, {7}}},
		{"=TREND(E1:E11,A1:D11,{2500,3,2,25})", near(158261.0956326)},
		{"=TREND({1;9;5;7},{0;4;2;3},{1,2})", ftest.Arr{{3, 5}}},
		{"=TREND(E1:E11,A1:D11,{1,2})", f.ErrRef},
		{"=GROWTH({33100,47300,69000,102000,150000,220000},{11,12,13,14,15,16})", ftest.Arr{{
			32618.203773539655, 47729.422614747666, 69841.30085621719, 102197.07337883205, 149542.48674004542, 218821.8762145949}}},
		{"=GROWTH({33100;47300;69000;102000;150000;220000},{11;12;13;14;15;16},{17;18})",
			ftest.Arr{{320196.718363471}, {468536.05418404634}}},
	})
}

func TestHypothesis(t *testing.T) {
	ftest.Run(t, registry(), regData, []ftest.Case{
		{"=T.TEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},2,1)", near(0.196016)},
		{"=TTEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},2,1)", near(0.196016)},
		{"=T.TEST({1,2},{1,2,3},2,1)", f.ErrNA},
		{"=T.TEST({1,2},{1,2},3,1)", f.ErrNum},
		{"=T.TEST({1,2},{1,2},2,4)", f.ErrNum},
		{"=T.TEST({1},{1,2},2,2)", f.ErrDiv0},
		{"=F.TEST({6,7,9,15,21},{20,28,31,38,40})", near(0.64831785)},
		{"=FTEST({6,7,9,15,21},{20,28,31,38,40})", near(0.64831785)},
		{"=F.TEST({1,1},{1,2})", f.ErrDiv0},
		{"=CHITEST({58,35;11,25;10,23},{45.35,47.65;17.56,18.44;16.09,16.91})", near(0.000308192)},
		{"=CHISQ.TEST({1,2},{1,2,3})", f.ErrNA},
		{"=CHISQ.TEST({10,20,30},{20,20,20})", 0.006737946999085467}, // χ² = 10, df 2: e^-5
		{"=Z.TEST({3,6,7,8,6,5,4,2,1,9},4)", near(0.090574)},
		{"=Z.TEST({3,6,7,8,6,5,4,2,1,9},6)", near(0.863043)},
		{"=ZTEST({3,6,7,8,6,5,4,2,1,9},6,1)", near(0.997786737)},
		{"=Z.TEST(G6,1)", f.ErrNA},
		{"=PROB({0,1,2,3},{0.2,0.3,0.1,0.4},2)", 0.1},
		{"=PROB({0,1,2,3},{0.2,0.3,0.1,0.4},1,3)", 0.8},
		{"=PROB({0,1,2,3},{0.2,0.3,0.1,0.3},1,3)", f.ErrNum},
		{"=PROB({0,1,2},{0.2,0.3,0.1,0.4},1)", f.ErrNA},
	})
}

// Microsoft's documented results (rounded) and the domain errors.
func TestDistributions(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=NORMDIST(42,40,1.5,TRUE)", near(0.9087888)},
		{"=NORMINV(0.908789,40,1.5)", near(42.000002)},
		{"=NORMSDIST(1.333333)", near(0.908788726)},
		{"=NORMSINV(0.908789)", near(1.3333347)},
		{"=NORM.DIST(1,0,0,TRUE)", f.ErrNum},
		{"=NORM.INV(0,0,1)", f.ErrNum},
		{"=NORM.INV(1,0,1)", f.ErrNum},
		{"=NORM.S.INV(0.5)", 0},
		{"=NORM.S.DIST(\"x\",TRUE)", f.ErrValue},
		{"=PHI(0.75)", near(0.301137432)},
		{"=GAUSS(2)", near(0.477249868)},
		{"=GAUSS(-2)", near(-0.477249868)},
		{"=TDIST(1.959999998,60,2)", near(0.054644930)},
		{"=TDIST(1.959999998,60,1)", near(0.027322465)},
		{"=TDIST(-1,60,1)", f.ErrNum},
		{"=TDIST(1,60,3)", f.ErrNum},
		{"=T.DIST(1,0,TRUE)", f.ErrNum},
		{"=T.DIST(0,5,TRUE)", 0.5},
		{"=T.DIST.2T(-1,5)", f.ErrNum},
		{"=T.DIST.RT(-1.959999998,60)", near(0.972677535)},
		{"=TINV(0.05464,60)", ftest.Approx{V: 1.96, Tol: 1e-4}},
		{"=T.INV.2T(1,10)", 0},
		{"=T.INV.2T(0,10)", f.ErrNum},
		{"=T.INV(0.5,10)", 0},
		{"=T.INV(1,10)", f.ErrNum},
		{"=CHIDIST(18.307,10)", near(0.050001)},
		{"=CHIINV(0.050001,10)", near(18.306973)},
		{"=CHISQ.DIST(-1,2,TRUE)", f.ErrNum},
		{"=CHISQ.DIST(1,0.5,TRUE)", f.ErrNum},
		{"=CHISQ.DIST(3,2.9,TRUE)", near(0.7768698399)}, // df truncated to 2: 1 - e^-1.5
		{"=CHISQ.INV(0,4)", 0},
		{"=CHISQ.INV(1.1,4)", f.ErrNum},
		{"=CHISQ.INV.RT(0,4)", f.ErrNum},
		{"=FDIST(15.2069,6,4)", near(0.01)},
		{"=FINV(0.01,6,4)", near(15.206865)},
		{"=F.DIST(-1,6,4,TRUE)", f.ErrNum},
		{"=F.DIST(1,0,4,TRUE)", f.ErrNum},
		{"=F.INV(0,6,4)", 0},
		{"=BINOMDIST(6,10,0.5,FALSE)", near(0.2050781)},
		{"=BINOM.DIST(11,10,0.5,FALSE)", f.ErrNum},
		{"=BINOM.DIST(1,10,1.5,FALSE)", f.ErrNum},
		{"=BINOM.DIST(0,10,0,FALSE)", 1},
		{"=BINOM.DIST(10,10,1,TRUE)", 1},
		{"=BINOM.DIST.RANGE(60,0.75,48)", near(0.083974967)},
		{"=BINOM.DIST.RANGE(60,0.75,45,50)", near(0.523629793)},
		{"=BINOM.DIST.RANGE(60,0.75,50,45)", f.ErrNum},
		{"=BINOM.INV(6,0.5,0.75)", 4},
		{"=CRITBINOM(6,0.5,0.75)", 4},
		{"=CRITBINOM(6,0.5,0)", 0},
		{"=CRITBINOM(6,0.5,1)", 6},
		{"=CRITBINOM(6,0.5,1.5)", f.ErrNum},
		{"=POISSON(2,5,TRUE)", near(0.124652)},
		{"=POISSON(2,5,FALSE)", near(0.084224)},
		{"=POISSON(0,0,FALSE)", 1},
		{"=POISSON(-1,5,FALSE)", f.ErrNum},
		{"=EXPONDIST(0.2,10,TRUE)", near(0.86466472)},
		{"=EXPON.DIST(0.2,0,TRUE)", f.ErrNum},
		{"=GAMMA(0)", f.ErrNum},
		{"=GAMMA(-2)", f.ErrNum},
		{"=GAMMA(5)", 24},
		{"=GAMMALN.PRECISE(4)", near(1.791759469)},
		{"=GAMMALN(0)", f.ErrNum},
		{"=GAMMADIST(10.00001131,9,2,TRUE)", near(0.068094)},
		{"=GAMMAINV(0.068094,9,2)", near(10.0000112)},
		{"=GAMMA.DIST(1,0,2,TRUE)", f.ErrNum},
		{"=GAMMA.DIST(0,1,2,FALSE)", 0.5},
		{"=BETADIST(2,8,10,1,3)", near(0.6854706)},
		{"=BETAINV(0.685470581,8,10,1,3)", near(2)},
		{"=BETA.DIST(0.5,1,1,TRUE)", 0.5},
		{"=BETA.DIST(4,8,10,TRUE,1,3)", f.ErrNum},
		{"=BETA.INV(0,8,10)", f.ErrNum},
		{"=BETA.INV(1,8,10,1,3)", 3},
		{"=LOGNORMDIST(4,3.5,1.2)", near(0.0390836)},
		{"=LOGINV(0.039084,3.5,1.2)", near(4.0000252)},
		{"=LOGNORM.DIST(0,3.5,1.2,TRUE)", f.ErrNum},
		{"=WEIBULL(105,20,100,TRUE)", near(0.929581)},
		{"=WEIBULL(105,20,100,FALSE)", near(0.035589)},
		{"=WEIBULL(-1,20,100,FALSE)", f.ErrNum},
		{"=HYPGEOMDIST(1,4,8,20)", near(0.363261)},
		{"=HYPGEOM.DIST(5,4,8,20,TRUE)", f.ErrNum},
		{"=HYPGEOM.DIST(4,4,8,20,TRUE)", near(1)},
		{"=NEGBINOMDIST(10,5,0.25)", near(0.05504866)},
		{"=NEGBINOM.DIST(10,0,0.25,TRUE)", f.ErrNum},
		{"=CONFIDENCE(0.05,2.5,50)", near(0.692952)},
		{"=CONFIDENCE.T(0.05,1,50)", near(0.284196855)},
		{"=CONFIDENCE.T(0.05,1,1)", f.ErrDiv0},
		{"=CONFIDENCE.NORM(1,1,50)", f.ErrNum},
		// Arguments lift over arrays.
		{"=NORM.S.DIST({0,1},TRUE)", ftest.Arr{{0.5, near(0.841344746)}}},
	})
}
