(setq src "abcdefghijklmnopqrstuvwxyz")
(setq n 0)
(setq i 1)

(while (loadb (+ src n))
  (setq n (+ n 1)))

(while (< i (- n 1))
  (out 1 (/ (+ (+ (loadb (+ src (- i 1))) (loadb (+ src i))) (loadb (+ src (+ i 1)))) 3))
  (setq i (+ i 1)))
(print 10)
