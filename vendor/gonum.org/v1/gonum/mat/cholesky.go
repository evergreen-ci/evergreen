// Copyright ©2013 The gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mat

import (
	"math"

	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/blas64"
	"gonum.org/v1/gonum/lapack/lapack64"
)

const (
	badTriangle = "mat: invalid triangle"
	badCholesky = "mat: invalid Cholesky factorization"
)

// Cholesky is a type for creating and using the Cholesky factorization of a
// symmetric positive definite matrix.
//
// Cholesky methods may only be called on a value that has been successfully
// initialized by a call to Factorize that has returned true. Calls to methods
// of an unsuccessful Cholesky factorization will panic.
type Cholesky struct {
	// The chol pointer must never be retained as a pointer outside the Cholesky
	// struct, either by returning chol outside the struct or by setting it to
	// a pointer coming from outside. The same prohibition applies to the data
	// slice within chol.
	chol *TriDense
	cond float64
}

// updateCond updates the condition number of the Cholesky decomposition. If
// norm > 0, then that norm is used as the norm of the original matrix A, otherwise
// the norm is estimated from the decomposition.
func (c *Cholesky) updateCond(norm float64) {
	n := c.chol.mat.N
	work := getFloats(3*n, false)
	defer putFloats(work)
	if norm < 0 {
		// This is an approximation. By the definition of a norm,
		//  |AB| <= |A| |B|.
		// Since A = U^T*U, we get for the condition number κ that
		//  κ(A) := |A| |A^-1| = |U^T*U| |A^-1| <= |U^T| |U| |A^-1|,
		// so this will overestimate the condition number somewhat.
		// The norm of the original factorized matrix cannot be stored
		// because of update possibilities.
		unorm := lapack64.Lantr(CondNorm, c.chol.mat, work)
		lnorm := lapack64.Lantr(CondNormTrans, c.chol.mat, work)
		norm = unorm * lnorm
	}
	sym := c.chol.asSymBlas()
	iwork := getInts(n, false)
	v := lapack64.Pocon(sym, norm, work, iwork)
	putInts(iwork)
	c.cond = 1 / v
}

// Cond returns the condition number of the factorized matrix.
func (c *Cholesky) Cond() float64 {
	return c.cond
}

// Factorize calculates the Cholesky decomposition of the matrix A and returns
// whether the matrix is positive definite. If Factorize returns false, the
// factorization must not be used.
func (c *Cholesky) Factorize(a Symmetric) (ok bool) {
	n := a.Symmetric()
	if c.isZero() {
		c.chol = NewTriDense(n, Upper, nil)
	} else {
		c.chol = NewTriDense(n, Upper, use(c.chol.mat.Data, n*n))
	}
	copySymIntoTriangle(c.chol, a)

	sym := c.chol.asSymBlas()
	work := getFloats(c.chol.mat.N, false)
	norm := lapack64.Lansy(CondNorm, sym, work)
	putFloats(work)
	_, ok = lapack64.Potrf(sym)
	if ok {
		c.updateCond(norm)
	} else {
		c.Reset()
	}
	return ok
}

// Reset resets the factorization so that it can be reused as the receiver of a
// dimensionally restricted operation.
func (c *Cholesky) Reset() {
	if !c.isZero() {
		c.chol.Reset()
	}
	c.cond = math.Inf(1)
}

// SetFromU sets the Cholesky decomposition from the given triangular matrix.
// SetFromU panics if t is not upper triangular. Note that t is copied into,
// not stored inside, the receiver.
func (c *Cholesky) SetFromU(t *TriDense) {
	n, kind := t.Triangle()
	if kind != Upper {
		panic("cholesky: matrix must be upper triangular")
	}
	if c.isZero() {
		c.chol = NewTriDense(n, Upper, nil)
	} else {
		c.chol = NewTriDense(n, Upper, use(c.chol.mat.Data, n*n))
	}
	c.chol.Copy(t)
	c.updateCond(-1)
}

// Clone makes a copy of the input Cholesky into the receiver, overwriting the
// previous value of the receiver. Clone does not place any restrictions on receiver
// shape. Clone panics if the input Cholesky is not the result of a valid decomposition.
func (c *Cholesky) Clone(chol *Cholesky) {
	if !chol.valid() {
		panic(badCholesky)
	}
	n := chol.Size()
	if c.isZero() {
		c.chol = NewTriDense(n, Upper, nil)
	} else {
		c.chol = NewTriDense(n, Upper, use(c.chol.mat.Data, n*n))
	}
	c.chol.Copy(chol.chol)
	c.cond = chol.cond
}

// Size returns the dimension of the factorized matrix.
func (c *Cholesky) Size() int {
	if !c.valid() {
		panic(badCholesky)
	}
	return c.chol.mat.N
}

// Det returns the determinant of the matrix that has been factorized.
func (c *Cholesky) Det() float64 {
	if !c.valid() {
		panic(badCholesky)
	}
	return math.Exp(c.LogDet())
}

// LogDet returns the log of the determinant of the matrix that has been factorized.
func (c *Cholesky) LogDet() float64 {
	if !c.valid() {
		panic(badCholesky)
	}
	var det float64
	for i := 0; i < c.chol.mat.N; i++ {
		det += 2 * math.Log(c.chol.mat.Data[i*c.chol.mat.Stride+i])
	}
	return det
}

// Solve finds the matrix m that solves A * m = b where A is represented
// by the Cholesky decomposition, placing the result in m.
func (c *Cholesky) Solve(m *Dense, b Matrix) error {
	if !c.valid() {
		panic(badCholesky)
	}
	n := c.chol.mat.N
	bm, bn := b.Dims()
	if n != bm {
		panic(ErrShape)
	}

	m.reuseAs(bm, bn)
	if b != m {
		m.Copy(b)
	}
	blas64.Trsm(blas.Left, blas.Trans, 1, c.chol.mat, m.mat)
	blas64.Trsm(blas.Left, blas.NoTrans, 1, c.chol.mat, m.mat)
	if c.cond > ConditionTolerance {
		return Condition(c.cond)
	}
	return nil
}

// SolveChol finds the matrix m that solves A * m = B where A and B are represented
// by their Cholesky decompositions a and b, placing the result in the receiver.
func (a *Cholesky) SolveChol(m *Dense, b *Cholesky) error {
	if !a.valid() || !b.valid() {
		panic(badCholesky)
	}
	bn := b.chol.mat.N
	if a.chol.mat.N != bn {
		panic(ErrShape)
	}

	m.reuseAsZeroed(bn, bn)
	m.Copy(b.chol.T())
	blas64.Trsm(blas.Left, blas.Trans, 1, a.chol.mat, m.mat)
	blas64.Trsm(blas.Left, blas.NoTrans, 1, a.chol.mat, m.mat)
	blas64.Trmm(blas.Right, blas.NoTrans, 1, b.chol.mat, m.mat)
	if a.cond > ConditionTolerance {
		return Condition(a.cond)
	}
	return nil
}

// SolveVec finds the vector v that solves A * v = b where A is represented
// by the Cholesky decomposition, placing the result in v.
func (c *Cholesky) SolveVec(v, b *VecDense) error {
	if !c.valid() {
		panic(badCholesky)
	}
	n := c.chol.mat.N
	vn := b.Len()
	if vn != n {
		panic(ErrShape)
	}
	if v != b {
		v.checkOverlap(b.mat)
	}
	v.reuseAs(n)
	if v != b {
		v.CopyVec(b)
	}
	blas64.Trsv(blas.Trans, c.chol.mat, v.mat)
	blas64.Trsv(blas.NoTrans, c.chol.mat, v.mat)
	if c.cond > ConditionTolerance {
		return Condition(c.cond)
	}
	return nil

}

// UTo extracts the n×n upper triangular matrix U from a Cholesky
// decomposition into dst and returns the result. If dst is nil a new
// TriDense is allocated.
//  A = U^T * U.
func (c *Cholesky) UTo(dst *TriDense) *TriDense {
	if !c.valid() {
		panic(badCholesky)
	}
	n := c.chol.mat.N
	if dst == nil {
		dst = NewTriDense(n, Upper, make([]float64, n*n))
	} else {
		dst.reuseAs(n, Upper)
	}
	dst.Copy(c.chol)
	return dst
}

// LTo extracts the n×n lower triangular matrix L from a Cholesky
// decomposition into dst and returns the result. If dst is nil a new
// TriDense is allocated.
//  A = L * L^T.
func (c *Cholesky) LTo(dst *TriDense) *TriDense {
	if !c.valid() {
		panic(badCholesky)
	}
	n := c.chol.mat.N
	if dst == nil {
		dst = NewTriDense(n, Lower, make([]float64, n*n))
	} else {
		dst.reuseAs(n, Lower)
	}
	dst.Copy(c.chol.TTri())
	return dst
}

// ToSym reconstructs the original positive definite matrix given its
// Cholesky decomposition into dst and returns the result. If dst is nil
// a new SymDense is allocated.
func (c *Cholesky) ToSym(dst *SymDense) *SymDense {
	if !c.valid() {
		panic(badCholesky)
	}
	n := c.chol.mat.N
	if dst == nil {
		dst = NewSymDense(n, make([]float64, n*n))
	} else {
		dst.reuseAs(n)
	}
	dst.SymOuterK(1, c.chol.T())
	return dst
}

// InverseTo computes the inverse of the matrix represented by its Cholesky
// factorization and stores the result into s. If the factorized
// matrix is ill-conditioned, a Condition error will be returned.
// Note that matrix inversion is numerically unstable, and should generally be
// avoided where possible, for example by using the Solve routines.
func (c *Cholesky) InverseTo(s *SymDense) error {
	if !c.valid() {
		panic(badCholesky)
	}
	// TODO(btracey): Replace this code with a direct call to Dpotri when it
	// is available.
	s.reuseAs(c.chol.mat.N)
	// If:
	//  chol(A) = U^T * U
	// Then:
	//  chol(A^-1) = S * S^T
	// where S = U^-1
	var t TriDense
	err := t.InverseTri(c.chol)
	s.SymOuterK(1, &t)
	return err
}

// SymRankOne performs a rank-1 update of the original matrix A and refactorizes
// its Cholesky factorization, storing the result into the receiver. That is, if
// in the original Cholesky factorization
//  U^T * U = A,
// in the updated factorization
//  U'^T * U' = A + alpha * x * x^T = A'.
//
// Note that when alpha is negative, the updating problem may be ill-conditioned
// and the results may be inaccurate, or the updated matrix A' may not be
// positive definite and not have a Cholesky factorization. SymRankOne returns
// whether the updated matrix A' is positive definite.
//
// SymRankOne updates a Cholesky factorization in O(n²) time. The Cholesky
// factorization computation from scratch is O(n³).
func (c *Cholesky) SymRankOne(orig *Cholesky, alpha float64, x *VecDense) (ok bool) {
	if !orig.valid() {
		panic(badCholesky)
	}
	n := orig.Size()
	if x.Len() != n {
		panic(ErrShape)
	}
	if orig != c {
		if c.isZero() {
			c.chol = NewTriDense(n, Upper, nil)
		} else if c.chol.mat.N != n {
			panic(ErrShape)
		}
		c.chol.Copy(orig.chol)
	}

	if alpha == 0 {
		return true
	}

	// Algorithms for updating and downdating the Cholesky factorization are
	// described, for example, in
	// - J. J. Dongarra, J. R. Bunch, C. B. Moler, G. W. Stewart: LINPACK
	//   Users' Guide. SIAM (1979), pages 10.10--10.14
	// or
	// - P. E. Gill, G. H. Golub, W. Murray, and M. A. Saunders: Methods for
	//   modifying matrix factorizations. Mathematics of Computation 28(126)
	//   (1974), Method C3 on page 521
	//
	// The implementation is based on LINPACK code
	// http://www.netlib.org/linpack/dchud.f
	// http://www.netlib.org/linpack/dchdd.f
	// and
	// https://icl.cs.utk.edu/lapack-forum/viewtopic.php?f=2&t=2646
	//
	// According to http://icl.cs.utk.edu/lapack-forum/archives/lapack/msg00301.html
	// LINPACK is released under BSD license.
	//
	// See also:
	// - M. A. Saunders: Large-scale Linear Programming Using the Cholesky
	//   Factorization. Technical Report Stanford University (1972)
	//   http://i.stanford.edu/pub/cstr/reports/cs/tr/72/252/CS-TR-72-252.pdf
	// - Matthias Seeger: Low rank updates for the Cholesky decomposition.
	//   EPFL Technical Report 161468 (2004)
	//   http://infoscience.epfl.ch/record/161468

	work := getFloats(n, false)
	defer putFloats(work)
	blas64.Copy(n, x.RawVector(), blas64.Vector{1, work})

	if alpha > 0 {
		// Compute rank-1 update.
		if alpha != 1 {
			blas64.Scal(n, math.Sqrt(alpha), blas64.Vector{1, work})
		}
		umat := c.chol.mat
		stride := umat.Stride
		for i := 0; i < n; i++ {
			// Compute parameters of the Givens matrix that zeroes
			// the i-th element of x.
			c, s, r, _ := blas64.Rotg(umat.Data[i*stride+i], work[i])
			if r < 0 {
				// Multiply by -1 to have positive diagonal
				// elemnts.
				r *= -1
				c *= -1
				s *= -1
			}
			umat.Data[i*stride+i] = r
			if i < n-1 {
				// Multiply the extended factorization matrix by
				// the Givens matrix from the left. Only
				// the i-th row and x are modified.
				blas64.Rot(n-i-1,
					blas64.Vector{1, umat.Data[i*stride+i+1 : i*stride+n]},
					blas64.Vector{1, work[i+1 : n]},
					c, s)
			}
		}
		c.updateCond(-1)
		return true
	}

	// Compute rank-1 downdate.
	alpha = math.Sqrt(-alpha)
	if alpha != 1 {
		blas64.Scal(n, alpha, blas64.Vector{1, work})
	}
	// Solve U^T * p = x storing the result into work.
	ok = lapack64.Trtrs(blas.Trans, c.chol.RawTriangular(), blas64.General{
		Rows:   n,
		Cols:   1,
		Stride: 1,
		Data:   work,
	})
	if !ok {
		// The original matrix is singular. Should not happen, because
		// the factorization is valid.
		panic(badCholesky)
	}
	norm := blas64.Nrm2(n, blas64.Vector{1, work})
	if norm >= 1 {
		// The updated matrix is not positive definite.
		return false
	}
	norm = math.Sqrt((1 + norm) * (1 - norm))
	cos := getFloats(n, false)
	defer putFloats(cos)
	sin := getFloats(n, false)
	defer putFloats(sin)
	for i := n - 1; i >= 0; i-- {
		// Compute parameters of Givens matrices that zero elements of p
		// backwards.
		cos[i], sin[i], norm, _ = blas64.Rotg(norm, work[i])
		if norm < 0 {
			norm *= -1
			cos[i] *= -1
			sin[i] *= -1
		}
	}
	umat := c.chol.mat
	stride := umat.Stride
	for i := n - 1; i >= 0; i-- {
		// Apply Givens matrices to U.
		// TODO(vladimir-ch): Use workspace to avoid modifying the
		// receiver in case an invalid factorization is created.
		blas64.Rot(n-i, blas64.Vector{1, work[i:n]}, blas64.Vector{1, umat.Data[i*stride+i : i*stride+n]}, cos[i], sin[i])
		if umat.Data[i*stride+i] == 0 {
			// The matrix is singular (may rarely happen due to
			// floating-point effects?).
			ok = false
		} else if umat.Data[i*stride+i] < 0 {
			// Diagonal elements should be positive. If it happens
			// that on the i-th row the diagonal is negative,
			// multiply U from the left by an identity matrix that
			// has -1 on the i-th row.
			blas64.Scal(n-i, -1, blas64.Vector{1, umat.Data[i*stride+i : i*stride+n]})
		}
	}
	if ok {
		c.updateCond(-1)
	} else {
		c.Reset()
	}
	return ok
}

func (c *Cholesky) isZero() bool {
	return c.chol == nil
}

func (c *Cholesky) valid() bool {
	return !c.isZero() && !c.chol.IsZero()
}
