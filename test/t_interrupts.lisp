(setq ch 0)
(setq ready 0)
(setq running 1)

(interrupt
    (setq ch (in 0))
    (setq ready 1))

(ei)

(while running
    (if ready
        (progn
            (di)
            (setq c ch)
            (setq ready 0)
            (ei)
            (if (== c 0)
                (setq running 0)
                (out 1 c)))))
