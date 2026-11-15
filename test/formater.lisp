(setq error_code 0)
(setq fmt_len 0)

(read ch)

(while (== fmt_len fmt_len)
    (if (== ch 10)
        (setq fmt_len (+ fmt_len 10000))
        (if (> fmt_len 32)
            (setq error_code -1)
            (setq fmt_len fmt_len)))

    (if (< fmt_len 10000)
        (read ch)
        (setq fmt_len fmt_len)))

(if (== error_code -1)
    (print -1)
    (setq error_code 0))

(if (== error_code 0)
    (read num_ch)
    (setq num_ch 0))

(setq is_negative 0)
(setq parsed_value 0)

(if (== num_ch 45)
    (setq is_negative 1)
    (if (== num_ch 43)
        (setq is_negative 0)
        (setq is_negative 0)))

(if (== num_ch 45)
    (read num_ch)
    (if (== num_ch 43)
        (read num_ch)
        (setq num_ch num_ch)))

(while (== (>= num_ch 48) (<= num_ch 57))
    (setq digit (- num_ch 48))
    (setq parsed_value (+ (* parsed_value 10) digit))

    (if (> parsed_value 2147483647)
        (setq error_code -1)
        (setq error_code error_code))

    (read num_ch))

(if (== is_negative 1)
    (setq parsed_value (- 0 parsed_value))
    (setq parsed_value parsed_value))

(if (== error_code -1)
    (print -1)
    (print parsed_value))
