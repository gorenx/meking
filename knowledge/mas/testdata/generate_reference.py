"""Regenerate public-API reference cases with make mas-reference."""

import json
from datetime import datetime, timedelta, timezone
from importlib.metadata import version
from pathlib import Path

from fsrs import Card, Rating, Scheduler, State


def timestamp(value):
    return value.isoformat().replace("+00:00", "Z")


def snapshot(card):
    return {
        "stability": card.stability or 0,
        "difficulty": card.difficulty or 0,
        "last_review": timestamp(card.last_review) if card.last_review else None,
    }


def main():
    assert version("fsrs") == "6.3.2"
    epoch = datetime(2026, 1, 1, 12, 0, 0, 123456, tzinfo=timezone.utc)
    baseline = list(Scheduler().parameters)
    alternate = baseline.copy()
    for index, value in {4: 8, 5: 0.1, 6: 0.4, 7: 0.2, 8: 1.2,
                         17: 0.8, 18: 0.4, 19: 0.2, 20: 0.5}.items():
        alternate[index] = value
    cases = []

    for profile, weights in (("default", baseline), ("alternate", alternate)):
        scheduler = Scheduler(parameters=weights, learning_steps=(),
                              relearning_steps=(), enable_fuzzing=False)

        def record(name, card, grade, at):
            before = snapshot(card)
            activation = scheduler.get_card_retrievability(card, at)
            updated, _ = scheduler.review_card(card, Rating(grade), at)
            cases.append({
                "name": f"{profile}/{name}", "parameters": weights,
                "before": before, "grade": grade, "at": timestamp(at),
                "activation": activation, "after": snapshot(updated),
                "probes": [{
                    "at": timestamp(at + timedelta(seconds=seconds)),
                    "activation": scheduler.get_card_retrievability(
                        updated, at + timedelta(seconds=seconds)),
                } for seconds in (-1, 0, 86399.999999, 86400, 172799.999999, 2592000)],
            })
            return updated

        for grade in range(1, 5):
            record(f"initial/{grade}", Card(), grade, epoch)
        for stability in (0.001, 0.2, 3, 1000):
            for difficulty in (1, 5, 10):
                for seconds in (0, 60, 86399.999999, 86400, 172799.999999, 2592000):
                    for grade in range(1, 5):
                        card = Card(state=State.Review, stability=stability,
                                    difficulty=difficulty, last_review=epoch)
                        record(f"transition/{stability}/{difficulty}/{seconds}/{grade}",
                               card, grade, epoch + timedelta(seconds=seconds))
        for name, grades in (("mixed", [3, 3, 1, 2, 4, 1, 4, 3] * 5),
                             ("dense_good", [3] * 40),
                             ("repeated_failure", [1] * 40)):
            card = Card()
            at = epoch
            for index, grade in enumerate(grades):
                seconds = (0, 60, 86400, 2592000)[index % 4] if name == "mixed" else 60
                at += timedelta(seconds=seconds)
                card = record(f"{name}/{index}", card, grade, at)

    destination = Path(__file__).with_name("reference.json")
    destination.write_text(json.dumps({
        "reference": "py-fsrs", "version": version("fsrs"),
        "commit": "9446cb06605c597a063aeee49f7d188d42e34dc2",
        "cases": cases,
    }, ensure_ascii=False, indent=2, allow_nan=False) + "\n", encoding="utf-8")
    print(f"Generated {len(cases)} reference cases: {destination}")


if __name__ == "__main__":
    main()
