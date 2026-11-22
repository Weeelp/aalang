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

(defun prime (n)
  (if (< n 2)
      0
      (progn
        (setq i 2)
        (setq p 1)
        (while (<= (* i i) n)
          (if (== (% n i) 0) (setq p 0))
          (setq i (+ i 1)))
        p)))

(setq n 0)
(setq c (getc))
(while (> c 47)
  (setq n (+ (* n 10) (- c 48)))
  (setq c (getc)))
(print (+ 48 (prime n)) 10)
