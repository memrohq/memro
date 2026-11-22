| Arm | Correct | Median input tokens | Mean input tokens | Median turns | Median s | Cost $ |
|---|---|---|---|---|---|---|
| none | 0/30 | 921 | 920 | 1.0 | 3.45 | 0.01 |
| gh | 29/30 | 16,558 | 17,619 | 3.0 | 15.7 | 0.06 |
| memro-raw | 26/30 | 16,222 | 16,857 | 2.0 | 6.2 | 0.04 |
| memro-facts | 23/30 | 14,318 | 15,983 | 3.0 | 6.1 | 0.03 |

memro-raw vs gh: fewer tokens on 15/30 (sign test p = 1), median ratio 0.99; correct where gh wasn't 0, the reverse 3 (McNemar p = 0.25)

memro-facts vs gh: fewer tokens on 19/30 (sign test p = 0.2), median ratio 0.91; correct where gh wasn't 0, the reverse 6 (McNemar p = 0.0312)
