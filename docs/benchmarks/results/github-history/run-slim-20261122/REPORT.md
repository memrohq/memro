| Arm | Correct | Median input tokens | Mean input tokens | Median turns | Median s | Cost $ |
|---|---|---|---|---|---|---|
| gh | 28/30 | 15,208 | 17,714 | 3.0 | 13.850000000000001 | 0.05 |
| memro-raw | 28/30 | 16,871 | 17,335 | 2.0 | 6.15 | 0.05 |
| memro-slim | 28/30 | 10,320 | 10,106 | 3.0 | 6.0 | 0.05 |

memro-raw vs gh: fewer tokens on 10/30 (sign test p = 0.0987), median ratio 1.20; correct where gh wasn't 1, the reverse 1 (McNemar p = 1)

memro-slim vs gh: fewer tokens on 26/30 (sign test p = 5.95e-05), median ratio 0.64; correct where gh wasn't 2, the reverse 2 (McNemar p = 1)
