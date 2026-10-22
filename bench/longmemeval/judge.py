"""Grade hypotheses with LongMemEval's official judge prompts.

    python judge.py runs/full                     # gpt-4o-mini on every answer
    python judge.py runs/full --model gpt-4o --sample 50   # cross-check a sample

Prompts are copied verbatim from LongMemEval (MIT License, Di Wu et al.):
https://github.com/xiaowu0162/LongMemEval/blob/main/src/evaluation/evaluate_qa.py
The official setup judges with gpt-4o; gpt-4o-mini is used here for cost, and
the --sample cross-check reports how often the two agree.
"""
import argparse
import concurrent.futures as cf
import json
import pathlib
import random
import threading

from common import BudgetExceeded, chat


def get_anscheck_prompt(task, question, answer, response, abstention=False):
    if not abstention:
        if task in ['single-session-user', 'single-session-assistant', 'multi-session']:
            template = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. \n\nQuestion: {}\n\nCorrect Answer: {}\n\nModel Response: {}\n\nIs the model response correct? Answer yes or no only."
            prompt = template.format(question, answer, response)
        elif task == 'temporal-reasoning':
            template = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response is equivalent to the correct answer or contains all the intermediate steps to get the correct answer, you should also answer yes. If the response only contains a subset of the information required by the answer, answer no. In addition, do not penalize off-by-one errors for the number of days. If the question asks for the number of days/weeks/months, etc., and the model makes off-by-one errors (e.g., predicting 19 days when the answer is 18), the model's response is still correct. \n\nQuestion: {}\n\nCorrect Answer: {}\n\nModel Response: {}\n\nIs the model response correct? Answer yes or no only."
            prompt = template.format(question, answer, response)
        elif task == 'knowledge-update':
            template = "I will give you a question, a correct answer, and a response from a model. Please answer yes if the response contains the correct answer. Otherwise, answer no. If the response contains some previous information along with an updated answer, the response should be considered as correct as long as the updated answer is the required answer.\n\nQuestion: {}\n\nCorrect Answer: {}\n\nModel Response: {}\n\nIs the model response correct? Answer yes or no only."
            prompt = template.format(question, answer, response)
        elif task == 'single-session-preference':
            template = "I will give you a question, a rubric for desired personalized response, and a response from a model. Please answer yes if the response satisfies the desired response. Otherwise, answer no. The model does not need to reflect all the points in the rubric. The response is correct as long as it recalls and utilizes the user's personal information correctly.\n\nQuestion: {}\n\nRubric: {}\n\nModel Response: {}\n\nIs the model response correct? Answer yes or no only."
            prompt = template.format(question, answer, response)
        else:
            raise NotImplementedError
    else:
        template = "I will give you an unanswerable question, an explanation, and a response from a model. Please answer yes if the model correctly identifies the question as unanswerable. The model could say that the information is incomplete, or some other information is given but the asked information is not.\n\nQuestion: {}\n\nExplanation: {}\n\nModel Response: {}\n\nDoes the model correctly identify the question as unanswerable? Answer yes or no only."
        prompt = template.format(question, answer, response)
    return prompt


def grade(row, model):
    prompt = get_anscheck_prompt(row["question_type"], row["question"], row["answer"], row["hypothesis"],
                                 abstention="_abs" in row["question_id"])
    out = chat(model, [{"role": "user", "content": prompt}], 10, len(prompt) / 3.0 * 10 / 1e6)
    return "yes" in out.lower()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("run_dir")
    ap.add_argument("--model", default="gpt-4o-mini")
    ap.add_argument("--sample", type=int, default=0)
    ap.add_argument("--workers", type=int, default=10)
    args = ap.parse_args()
    run = pathlib.Path(args.run_dir)
    rows = [json.loads(l) for l in (run / "hyp.jsonl").read_text().splitlines() if l.strip()]
    if args.sample:
        rows = random.Random(0).sample(rows, min(args.sample, len(rows)))
    out = run / f"eval-{args.model}.jsonl"
    done = {json.loads(l)["question_id"] for l in out.read_text().splitlines()} if out.exists() else set()
    todo = [r for r in rows if r["question_id"] not in done]
    print(f"{len(done)} already graded, {len(todo)} to grade -> {out}", flush=True)

    lock = threading.Lock()
    stop = threading.Event()

    def work(row):
        if stop.is_set():
            return
        try:
            ok = grade(row, args.model)
        except BudgetExceeded as e:
            stop.set()
            print(f"STOP: {e}", flush=True)
            return
        except Exception as e:
            print(f"Error grading {row['question_id']}: {e}", flush=True)
            return
        with lock:
            with open(out, "a") as f:
                f.write(json.dumps({"question_id": row["question_id"], "label": ok}) + "\n")
                f.flush()

    with cf.ThreadPoolExecutor(args.workers) as ex:
        list(ex.map(work, todo))
    print(f"graded -> {out}")


if __name__ == "__main__":
    main()
