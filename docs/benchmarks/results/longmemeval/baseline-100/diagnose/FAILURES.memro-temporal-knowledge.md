# Failure analysis: baseline-100

22 wrong answers. "Evidence" counts the exchanges LongMemEval marks as holding the answer (has_answer) that were in the prompt the answering model saw.

| Evidence in prompt | Failures |
|---|---|
| all evidence shown | 12 |
| partial evidence | 7 |
| no evidence shown | 2 |
| abstention question | 1 |

- Abstained ("don't know" etc.): 7
- Replay matched the run's retrieval: 22/22

## Questions

### `2133c1b5` · knowledge-update · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/10/15 (Sun) 08:39): How long have I been living in my current apartment in Harajuku?
- **Expected**: 3 months
- **Answered**: You have been living in your current apartment in Harajuku for about 6 months and 4 days.
- Prompt: `prompts/2133c1b5.txt` (20 items from 5 sessions, 40,099 chars)

### `69fee5aa` · knowledge-update · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/06/09 (Fri) 11:14): How many pre-1920 American coins do I have in my collection?
- **Expected**: 38
- **Answered**: You have 37 pre-1920 American coins in your collection.
- Prompt: `prompts/69fee5aa.txt` (20 items from 5 sessions, 51,824 chars)

### `f685340e` · knowledge-update · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/07/31 (Mon) 11:43): How often do I play tennis with my friends at the local park previously? How often do I play now?
- **Expected**: Previously, you play tennis with your friends at the local park every week (on Sunday). Currently, you play tennis every other week (on Sunday).
- **Answered**: You previously played tennis with your friends at the local park every other week, and you are still playing at that same frequency now.
- Prompt: `prompts/f685340e.txt` (20 items from 5 sessions, 47,682 chars)

### `1192316e` · multi-session · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 19:00): What is the total time it takes I to get ready and commute to work?
- **Expected**: an hour and a half
- **Answered**: I don't know the duration of your commute to work.
- Prompt: `prompts/1192316e.txt` (25 items from 9 sessions, 67,050 chars)

### `28dc39ac` · multi-session · partial evidence (4/5 exchanges, 5/5 sessions)

- **Question** (2023/05/30 (Tue) 23:42): How many hours have I spent playing games in total?
- **Expected**: 140 hours
- **Answered**: You have spent a total of 40 hours playing games.
- Prompt: `prompts/28dc39ac.txt` (21 items from 6 sessions, 60,567 chars)

### `2b8f3739` · multi-session · all evidence shown (3/3 exchanges, 3/3 sessions)

- **Question** (2023/06/01 (Thu) 23:37): What is the total amount of money I earned from selling my products at the markets?
- **Expected**: $495
- **Answered**: $345.
- Prompt: `prompts/2b8f3739.txt` (22 items from 5 sessions, 59,507 chars)

### `73d42213` · multi-session · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 22:10): What time did I reach the clinic on Monday?
- **Expected**: 9:00 AM
- **Answered**: The exact time you reached the clinic is not specified, but you left home at 7 AM.
- Prompt: `prompts/73d42213.txt` (17 items from 5 sessions, 37,651 chars)

### `88432d0a` · multi-session · partial evidence (3/4 exchanges, 4/4 sessions)

- **Question** (2023/05/30 (Tue) 23:45): How many times did I bake something in the past two weeks?
- **Expected**: 4
- **Answered**: You baked something five times in the past two weeks.
- Prompt: `prompts/88432d0a.txt` (22 items from 7 sessions, 53,375 chars)

### `8e91e7d9` · multi-session · partial evidence (1/2 exchanges, 1/2 sessions)

- **Question** (2023/05/30 (Tue) 09:50): What is the total number of siblings I have?
- **Expected**: 4
- **Answered**: You have a total of 1 sibling.
- Prompt: `prompts/8e91e7d9.txt` (24 items from 5 sessions, 44,831 chars)

### `92a0aa75` · multi-session · partial evidence (1/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 21:25): How long have I been working in my current role?
- **Expected**: 1 year and 5 months
- **Answered**: You have been working in your current role as a Senior Marketing Specialist for 2 years and 4 months.
- Prompt: `prompts/92a0aa75.txt` (21 items from 7 sessions, 47,941 chars)

### `d682f1a2` · multi-session · all evidence shown (3/3 exchanges, 3/3 sessions)

- **Question** (2023/05/30 (Tue) 23:30): How many different types of food delivery services have I used recently?
- **Expected**: 3
- **Answered**: You have used two different types of food delivery services recently.
- Prompt: `prompts/d682f1a2.txt` (26 items from 7 sessions, 61,416 chars)

### `d905b33f` · multi-session · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 21:08): What percentage discount did I get on the book from my favorite author?
- **Expected**: 20%
- **Answered**: I don't know the percentage discount without the final price you paid for the book.
- Prompt: `prompts/d905b33f.txt` (29 items from 11 sessions, 67,021 chars)

### `e3038f8c` · multi-session · partial evidence (3/4 exchanges, 4/4 sessions)

- **Question** (2023/05/30 (Tue) 20:48): How many rare items do I have in total?
- **Expected**: 99
- **Answered**: You have a total of 94 rare items.
- Prompt: `prompts/e3038f8c.txt` (24 items from 6 sessions, 71,225 chars)

### `gpt4_372c3eed_abs` · multi-session · abstention question (0/0 exchanges, 2/3 sessions)

- **Question** (2023/08/20 (Sun) 23:54): How many years in total did I spend in formal education from high school to the completion of my Master's degree?
- **Expected**: The information provided is not enough. You mentioned 4 years in high school (2010-2014), 2 years at PCC (2014-2016), and 4 years at UCLA (2016-2020). But you didn't mention the number of years you spend getting the Master's degree
- **Answered**: You spent a total of 10 years in formal education from high school to the completion of your Master's degree.
- Prompt: `prompts/gpt4_372c3eed_abs.txt` (28 items from 7 sessions, 45,571 chars)

### `0edc2aef` · single-session-preference · all evidence shown (1/1 exchanges, 1/1 sessions)

- **Question** (2023/05/30 (Tue) 20:23): Can you suggest a hotel for my upcoming trip to Miami?
- **Expected**: The user would prefer suggestions of hotels in Miami that offer great views, possibly of the ocean or the city skyline, and have unique features such as a rooftop pool or a hot tub on the balcony. They may not prefer suggestions of basic or budget hotels without these features.
- **Answered**: I don't know.
- Prompt: `prompts/0edc2aef.txt` (23 items from 4 sessions, 51,409 chars)

### `71017277` · temporal-reasoning · all evidence shown (1/1 exchanges, 1/1 sessions)

- **Question** (2023/03/09 (Thu) 15:47): I received a piece of jewelry last Saturday from whom?
- **Expected**: my aunt
- **Answered**: I don't know.
- Prompt: `prompts/71017277.txt` (22 items from 7 sessions, 53,448 chars)

### `9a707b82` · temporal-reasoning · no evidence shown (0/1 exchanges, 2/2 sessions)

- **Question** (2022/04/12 (Tue) 22:57): I mentioned cooking something for my friend a couple of days ago. What was it?
- **Expected**: a chocolate cake
- **Answered**: You mentioned cooking chicken soup for your mom.
- Prompt: `prompts/9a707b82.txt` (23 items from 6 sessions, 48,070 chars)

### `gpt4_21adecb5` · temporal-reasoning · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/07/01 (Sat) 08:16): How many months passed between the completion of my undergraduate degree and the submission of my master's thesis?
- **Expected**: 6 months
- **Answered**: 9 months.
- Prompt: `prompts/gpt4_21adecb5.txt` (21 items from 9 sessions, 61,470 chars)

### `gpt4_8279ba03` · temporal-reasoning · no evidence shown (0/1 exchanges, 0/1 sessions)

- **Question** (2023/03/25 (Sat) 18:26): What kitchen appliance did I buy 10 days ago?
- **Expected**: a smoker
- **Answered**: I don't know.
- Prompt: `prompts/gpt4_8279ba03.txt` (27 items from 9 sessions, 52,995 chars)

### `gpt4_93159ced` · temporal-reasoning · partial evidence (2/3 exchanges, 1/2 sessions)

- **Question** (2023/05/25 (Thu) 19:17): How long have I been working before I started my current job at NovaTech?
- **Expected**: 4 years and 9 months
- **Answered**: I don't know the exact duration you worked before starting at NovaTech, as the graduation date is not provided in the excerpts.
- Prompt: `prompts/gpt4_93159ced.txt` (23 items from 5 sessions, 49,700 chars)

### `gpt4_a1b77f9c` · temporal-reasoning · partial evidence (5/6 exchanges, 5/6 sessions)

- **Question** (2022/04/30 (Sat) 13:22): How many weeks in total do I spent on reading 'The Nightingale' and listening to 'Sapiens: A Brief History of Humankind' and 'The Power'?
- **Expected**: 2 weeks for 'The Nightingale', 4 weeks for 'Sapiens: A Brief History of Humankind', and 2 weeks for 'The Power', so a total of 8 weeks.
- **Answered**: You spent a total of 34 weeks reading 'The Nightingale' and listening to 'Sapiens: A
- Prompt: `prompts/gpt4_a1b77f9c.txt` (24 items from 5 sessions, 72,015 chars)

### `gpt4_e414231f` · temporal-reasoning · all evidence shown (1/1 exchanges, 2/2 sessions)

- **Question** (2023/03/21 (Tue) 21:43): Which bike did I fixed or serviced the past weekend?
- **Expected**: road bike
- **Answered**: You fixed the flat tire on your mountain bike a few weeks ago, but there is no mention of servicing any bike this past weekend.
- Prompt: `prompts/gpt4_e414231f.txt` (23 items from 6 sessions, 49,135 chars)

