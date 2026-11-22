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

(setq buf "b                               ")
(setq n 0)
(setq c 0)

(print "What is your name?\n")
(setq c (getc))
(while (> c 10)
  (if (< n 31)
      (progn (storeb (+ buf n) c) (setq n (+ n 1))))
  (setq c (getc)))
(storeb (+ buf n) 0)
(print "Hello, ")
(puts buf)
(print "!\n")
