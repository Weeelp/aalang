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

(setq c 1)
(while c
  (setq c (getc))
  (if c (out 1 c)))
