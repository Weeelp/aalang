(defun fact (n)
  (if (< n 2)
      1
      (* n (fact (- n 1)))))

(defun fib (n)
  (if (< n 2)
      n
      (+ (fib (- n 1)) (fib (- n 2)))))

(defun print-int (n)
  (if (< n 0)
      (progn (out 1 45) (print-int (- 0 n)))
      (progn
        (if (> n 9) (print-int (/ n 10)))
        (out 1 (+ 48 (% n 10))))))

(print-int (fact 5))
(print 10)
(print-int (fib 10))
(print 10)
