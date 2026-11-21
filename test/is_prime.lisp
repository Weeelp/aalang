(setq head 0)
(setq tail 0)
(setq eof 0)
(setq qbuf "q               ")

(interrupt
  (storeb (+ qbuf tail) (in 0))
  (setq tail (% (+ tail 1) 16)))

(defun getc ()
  (if eof
      0
      (progn
        (while (== head tail))
        (setq gc_v (loadb (+ qbuf head)))
        (setq head (% (+ head 1) 16))
        (if (== gc_v 0) (setq eof 1))
        gc_v)))

(ei)

(setq n 0)
(setq c (getc))
(while (> c 47)
    (setq n (+ (* n 10) (- c 48)))
    (setq c (getc)))

(setq is_prime 1)
(setq i 2)

(if (< n 2)
    (setq is_prime 0)
    (setq is_prime 1))

(while (<= (* i i) n)
    (setq temp (/ n i))
    (setq temp (* temp i))
    (setq remainder (- n temp))

    (if (== remainder 0)
        (setq is_prime 0)
        (setq is_prime is_prime))

    (if (== is_prime 0)
        (setq i (+ n 1))
        (setq i i))

    (setq i (+ i 1)))

(print (+ 48 is_prime))
