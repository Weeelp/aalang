(setq ch 0)
(setq cnt 0)
(setq done 0)
(setq k 0)

(interrupt
  (setq ch (in 0))
  (setq cnt (+ cnt 1))
  (if (== ch 0) (setq done 1))
  (setq k 0)
  (while (< k 40)
    (setq k (+ k 1))))

(ei)
(while (== done 0))
(print (+ 48 cnt) 10)
