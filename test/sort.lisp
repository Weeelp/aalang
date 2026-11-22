(setq head 0)
(setq tail 0)
(setq eof 0)
(setq qbuf "q                               ")

(interrupt
  (storeb (+ qbuf tail) (in 0))
  (setq tail (% (+ tail 1) 32)))

(defun getc ()
  (if eof
      0
      (progn
        (while (== head tail))
        (setq ch (loadb (+ qbuf head)))
        (setq head (% (+ head 1) 32))
        (if (== ch 0) (setq eof 1))
        ch)))

(ei)

(setq arr "a                                                                   ")
(setq n 0)
(setq x 0)
(setq i 0)
(setq j 0)
(setq a 0)
(setq b 0)

(defun read-int ()
  (setq v 0)
  (setq ch (getc))
  (while (>= ch 48)
    (setq v (+ (* v 10) (- ch 48)))
    (setq ch (getc)))
  v)

(defun print-int (n)
  (if (< n 0)
      (progn (out 1 45) (print-int (- 0 n)))
      (progn
        (if (> n 9) (print-int (/ n 10)))
        (out 1 (+ 48 (% n 10))))))

(setq x (read-int))
(while (> x 0)
  (if (< n 16)
      (progn (store (+ arr (* n 4)) x) (setq n (+ n 1))))
  (setq x (read-int)))

(setq i 0)
(while (< i n)
  (setq j 0)
  (while (< j (- (- n i) 1))
    (setq a (load (+ arr (* j 4))))
    (setq b (load (+ arr (* (+ j 1) 4))))
    (if (> a b)
        (progn
          (store (+ arr (* j 4)) b)
          (store (+ arr (* (+ j 1) 4)) a)))
    (setq j (+ j 1)))
  (setq i (+ i 1)))

(setq i 0)
(while (< i n)
  (print-int (load (+ arr (* i 4))))
  (print 10)
  (setq i (+ i 1)))
