# Failure analysis: arm-rag-100

19 wrong answers. "Evidence" counts the exchanges LongMemEval marks as holding the answer (has_answer) that were in the prompt the answering model saw.

| Evidence in prompt | Failures |
|---|---|
| all evidence shown | 9 |
| partial evidence | 7 |
| no evidence shown | 2 |
| abstention question | 1 |

- Abstained ("don't know" etc.): 3
- Replay matched the run's retrieval: 17/19

## Questions

### `69fee5aa` · knowledge-update · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/06/09 (Fri) 11:14): How many pre-1920 American coins do I have in my collection?
- **Expected**: 38
- **Answered**: You have 37 pre-1920 American coins in your collection.
- Prompt: `prompts/69fee5aa.txt` (15 items from 5 sessions, 39,872 chars)

### `f685340e` · knowledge-update · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/07/31 (Mon) 11:43): How often do I play tennis with my friends at the local park previously? How often do I play now?
- **Expected**: Previously, you play tennis with your friends at the local park every week (on Sunday). Currently, you play tennis every other week (on Sunday).
- **Answered**: You previously played tennis with your friends at the local park every other week, and you still play now at the same frequency.
- Prompt: `prompts/f685340e.txt` (15 items from 5 sessions, 32,527 chars)

### `10d9b85a` · multi-session · partial evidence (1/2 exchanges, 2/2 sessions)

- **Question** (2023/05/01 (Mon) 23:15): How many days did I spend attending workshops, lectures, and conferences in April?
- **Expected**: 3 days
- **Answered**: 1 day.
- Prompt: `prompts/10d9b85a.txt` (19 items from 9 sessions, 56,872 chars)

### `28dc39ac` · multi-session · partial evidence (4/5 exchanges, 5/5 sessions)

- **Question** (2023/05/30 (Tue) 23:42): How many hours have I spent playing games in total?
- **Expected**: 140 hours
- **Answered**: You have spent a total of 40 hours playing games.
- Prompt: `prompts/28dc39ac.txt` (18 items from 6 sessions, 53,440 chars)

### `8e91e7d9` · multi-session · partial evidence (1/2 exchanges, 1/2 sessions)

- **Question** (2023/05/30 (Tue) 09:50): What is the total number of siblings I have?
- **Expected**: 4
- **Answered**: You have 1 sibling (a brother).
- Prompt: `prompts/8e91e7d9.txt` (15 items from 5 sessions, 24,269 chars)

### `92a0aa75` · multi-session · partial evidence (1/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 21:25): How long have I been working in my current role?
- **Expected**: 1 year and 5 months
- **Answered**: You have been working in your current role as a Senior Marketing Specialist for 2 years and 4 months.
- Prompt: `prompts/92a0aa75.txt` (17 items from 7 sessions, 41,147 chars)

### `d682f1a2` · multi-session · all evidence shown (3/3 exchanges, 3/3 sessions)

- **Question** (2023/05/30 (Tue) 23:30): How many different types of food delivery services have I used recently?
- **Expected**: 3
- **Answered**: 2 different types of food delivery services.
- Prompt: `prompts/d682f1a2.txt` (20 items from 7 sessions, 48,579 chars)

### `e3038f8c` · multi-session · partial evidence (2/4 exchanges, 4/4 sessions)

- **Question** (2023/05/30 (Tue) 20:48): How many rare items do I have in total?
- **Expected**: 99
- **Answered**: You have at least 69 rare items (57 rare records and 12 rare figurines), but the total could be higher if you include your rare coins and rare books.
- Prompt: `prompts/e3038f8c.txt` (17 items from 6 sessions, 49,487 chars)

### `ef66a6e5` · multi-session · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/05/30 (Tue) 20:36): How many sports have I played competitively in the past?
- **Expected**: two
- **Answered**: 3 sports.
- Prompt: `prompts/ef66a6e5.txt` (15 items from 5 sessions, 41,195 chars)

### `gpt4_372c3eed_abs` · multi-session · abstention question (0/0 exchanges, 2/3 sessions)

- **Question** (2023/08/20 (Sun) 23:54): How many years in total did I spend in formal education from high school to the completion of my Master's degree?
- **Expected**: The information provided is not enough. You mentioned 4 years in high school (2010-2014), 2 years at PCC (2014-2016), and 4 years at UCLA (2016-2020). But you didn't mention the number of years you spend getting the Master's degree
- **Answered**: You spent a total of 10 years in formal education from high school to the completion of your Master's degree.
- Prompt: `prompts/gpt4_372c3eed_abs.txt` (18 items from 7 sessions, 41,076 chars)

### `0edc2aef` · single-session-preference · all evidence shown (1/1 exchanges, 1/1 sessions)

- **Question** (2023/05/30 (Tue) 20:23): Can you suggest a hotel for my upcoming trip to Miami?
- **Expected**: The user would prefer suggestions of hotels in Miami that offer great views, possibly of the ocean or the city skyline, and have unique features such as a rooftop pool or a hot tub on the balcony. They may not prefer suggestions of basic or budget hotels without these features.
- **Answered**: I don't have specific information about your preferences for a hotel in Miami, but I can suggest options like The Fontainebleau Miami Beach, The Setai, W Miami, and EAST Miami. If you have specific preferences, please let me know!
- Prompt: `prompts/0edc2aef.txt` (15 items from 4 sessions, 32,550 chars)

### `caf03d32` · single-session-preference · all evidence shown (2/2 exchanges, 1/1 sessions)

- **Question** (2023/05/30 (Tue) 23:44): I've been struggling with my slow cooker recipes. Any advice on getting better results?
- **Expected**: The user would prefer responses that provide tips and advice specifically tailored to their slow cooker experiences, utilizing their recent success with beef stew and interest in making yogurt in the slow cooker. They might not prefer general slow cooker recipes or advice unrelated to their specific experiences and interests.
- **Answered**: Based on your previous sessions, you've been exploring various slow cooker recipes, including vegetarian and vegan options, and you've recently made a delicious beef stew. Here are some tailored tips to help you improve your slow cooker results:

1. **Layering Ingredients**: Place denser ingredients like root vegetables (carrots, potatoes) at the bottom of the slow cooker, as they take longer to c
- Prompt: `prompts/caf03d32.txt` (17 items from 5 sessions, 36,047 chars)

### `71017277` · temporal-reasoning · all evidence shown (1/1 exchanges, 1/1 sessions)

- **Question** (2023/03/09 (Thu) 15:47): I received a piece of jewelry last Saturday from whom?
- **Expected**: my aunt
- **Answered**: I don't know.
- Prompt: `prompts/71017277.txt` (16 items from 7 sessions, 35,980 chars)

### `9a707b82` · temporal-reasoning · no evidence shown (0/1 exchanges, 2/2 sessions)

- **Question** (2022/04/12 (Tue) 22:57): I mentioned cooking something for my friend a couple of days ago. What was it?
- **Expected**: a chocolate cake
- **Answered**: You were thinking of making chicken soup for your mom.
- Prompt: `prompts/9a707b82.txt` (17 items from 6 sessions, 35,642 chars)

### `gpt4_21adecb5` · temporal-reasoning · all evidence shown (2/2 exchanges, 2/2 sessions)

- **Question** (2023/07/01 (Sat) 08:16): How many months passed between the completion of my undergraduate degree and the submission of my master's thesis?
- **Expected**: 6 months
- **Answered**: 9 months passed between the completion of your undergraduate degree and the submission of your master's thesis.
- Prompt: `prompts/gpt4_21adecb5.txt` (20 items from 9 sessions, 58,344 chars)

### `gpt4_8279ba03` · temporal-reasoning · no evidence shown (0/1 exchanges, 0/1 sessions)

- **Question** (2023/03/25 (Sat) 18:26): What kitchen appliance did I buy 10 days ago?
- **Expected**: a smoker
- **Answered**: I don't know.
- Prompt: `prompts/gpt4_8279ba03.txt` (19 items from 9 sessions, 37,684 chars)

### `gpt4_93159ced` · temporal-reasoning · partial evidence (2/3 exchanges, 1/2 sessions)

- **Question** (2023/05/25 (Thu) 19:17): How long have I been working before I started my current job at NovaTech?
- **Expected**: 4 years and 9 months
- **Answered**: I don't know how long you worked before starting your current job at NovaTech without knowing your graduation date or when you started your first job.
- Prompt: `prompts/gpt4_93159ced.txt` (16 items from 5 sessions, 36,404 chars)

### `gpt4_a1b77f9c` · temporal-reasoning · partial evidence (5/6 exchanges, 5/6 sessions)

- **Question** (2022/04/30 (Sat) 13:22): How many weeks in total do I spent on reading 'The Nightingale' and listening to 'Sapiens: A Brief History of Humankind' and 'The Power'?
- **Expected**: 2 weeks for 'The Nightingale', 4 weeks for 'Sapiens: A Brief History of Humankind', and 2 weeks for 'The Power', so a total of 8 weeks.
- **Answered**: You spent a total of 34 weeks reading 'The Nightingale' and listening to 'Sapiens: A
- Prompt: `prompts/gpt4_a1b77f9c.txt` (17 items from 5 sessions, 51,396 chars)

### `gpt4_e414231f` · temporal-reasoning · all evidence shown (1/1 exchanges, 2/2 sessions)

- **Question** (2023/03/21 (Tue) 21:43): Which bike did I fixed or serviced the past weekend?
- **Expected**: road bike
- **Answered**: You fixed the flat tire on your mountain bike a few weeks ago, but there is no mention of fixing or servicing any bike this past weekend.
- Prompt: `prompts/gpt4_e414231f.txt` (15 items from 6 sessions, 41,157 chars)

