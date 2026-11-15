(setq n 0)
(read c)
(while (> c 47)
    (setq n (+ (* n 10) (- c 48)))
    (read c))

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

(print is_prime)
