"""Графики результатов замеров по CSV из docs/results/data.

Запуск: python3 docs/results/figures/make_figures.py (нужен matplotlib).
Замеры: go run ./cmd/bench (параметры — в docs/results/README.md).
"""
import csv
import statistics
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402

HERE = Path(__file__).parent
DATA = HERE.parent / "data"

# Цвета: категориальные слоты 1–2 проверенной палитры, текст — нейтральные чернила.
BLUE, ORANGE = "#2a78d6", "#eb6834"
INK, INK2, GRID = "#0b0b0b", "#52514e", "#e4e3df"
LOWER_BOUND = 12_000  # неустранимый день с одной парой 

plt.rcParams.update({
    "font.family": "DejaVu Sans", "font.size": 10,
    "axes.edgecolor": INK2, "axes.labelcolor": INK, "xtick.color": INK2, "ytick.color": INK2,
    "axes.spines.top": False, "axes.spines.right": False,
    "axes.grid": True, "grid.color": GRID, "grid.linewidth": 0.8, "axes.axisbelow": True,
    "figure.dpi": 100, "savefig.dpi": 300, "savefig.bbox": "tight",
})


def rows(name):
    with open(DATA / name, encoding="utf-8") as f:
        return list(csv.DictReader(f))


def thousands(x, _pos=None):
    return f"{x / 1000:.0f} тыс."


def lower_bound_line(ax, horizontal=True):
    kw = dict(color=INK2, linestyle="--", linewidth=1)
    if horizontal:
        ax.axhline(LOWER_BOUND, **kw)
    else:
        ax.axvline(LOWER_BOUND, **kw)


def fig_budget():
    """Бюджет: критерий ILS в зависимости от бюджета времени."""
    data = {}
    with open(DATA / "budget.csv", encoding="utf-8") as f:
        for line in f:
            p = line.strip().split(",")
            data.setdefault(int(p[0].rstrip("s")), []).append(int(p[4]))
    budgets = sorted(data)
    fig, ax = plt.subplots(figsize=(6.3, 3.6))
    for b in budgets:
        ax.scatter([b] * len(data[b]), data[b], s=36, color=BLUE, alpha=0.45,
                   edgecolor="white", linewidth=1, zorder=3)
    med = [statistics.median(data[b]) for b in budgets]
    ax.plot(budgets, med, color=BLUE, linewidth=2, marker="o", markersize=7, zorder=4,
            markeredgecolor="white", markeredgewidth=1.5)
    for b, m in zip(budgets, med):
        ax.annotate(f"{m:,.0f}".replace(",", " "), (b, m), textcoords="offset points",
                    xytext=(8, 6), color=INK, fontsize=9)
    lower_bound_line(ax)
    ax.text(budgets[0], LOWER_BOUND, "нижняя граница L = 12 000", color=INK2, fontsize=9,
            ha="left", va="top")
    ax.set_xticks(budgets)
    ax.set_xlabel("Бюджет времени, с")
    ax.set_ylabel("Критерий F")
    ax.yaxis.set_major_formatter(thousands)
    ax.set_ylim(0, max(max(v) for v in data.values()) * 1.08)
    ax.set_xlim(5, 100)
    ax.set_title("Медиана (линия) и отдельные прогоны (точки), ILS после DSatur",
                 fontsize=10, color=INK2, loc="left")
    fig.savefig(HERE / "budget.png")
    plt.close(fig)


def dot_rows(ax, groups, colors):
    """Горизонтальная точечная диаграмма: строка на группу, точка на прогон, штрих — медиана."""
    for y, (label, values) in enumerate(groups):
        color = colors[y]
        ax.scatter(values, [y] * len(values), s=40, color=color, alpha=0.55,
                   edgecolor="white", linewidth=1, zorder=3)
        m = statistics.median(values)
        ax.plot([m, m], [y - 0.28, y + 0.28], color=color, linewidth=2, zorder=4)
        ax.annotate(f"{m:,.0f}".replace(",", " "), (m, y - 0.28), textcoords="offset points",
                    xytext=(3, 0), ha="left", va="bottom", fontsize=8, color=INK,
                    bbox=dict(boxstyle="square,pad=0.1", fc="white", ec="none"), zorder=5)
    ax.set_yticks(range(len(groups)))
    ax.set_yticklabels([g[0] for g in groups])
    ax.invert_yaxis()
    ax.grid(axis="y", visible=False)
    ax.xaxis.set_major_formatter(thousands)
    lower_bound_line(ax, horizontal=False)


def fig_methods():
    """Методы: методы улучшения при двух построениях (одиночный запуск, 45 с)."""
    r = rows("main.csv")
    names = {"hillclimb": "ILS", "sa": "Имитация отжига", "tabu": "Табу-поиск",
             "ga": "Генетический", "lns": "LNS"}
    groups, colors = [], []
    for construct, color, cname in (("dsatur", BLUE, "DSatur"), ("teacher", ORANGE, "по преп.")):
        for m in names:
            v = [int(x["score"]) for x in r if x["construct"] == construct and x["method"] == m]
            groups.append((f"{names[m]} · {cname}", v))
            colors.append(color)
    fig, ax = plt.subplots(figsize=(6.5, 5.2))
    dot_rows(ax, groups, colors)
    ax.axhline(4.5, color=GRID, linewidth=1)
    ax.set_xlim(0, 56_000)
    ax.text(LOWER_BOUND, -0.6, " L = 12 000", color=INK2, fontsize=8, va="bottom")
    ax.set_xlabel("Критерий F (меньше — лучше)")
    ax.set_title("Точки — прогоны, штрих — медиана; синим — после DSatur, оранжевым — по преподавателям",
                 fontsize=9, color=INK2, loc="left")
    fig.savefig(HERE / "methods.png")
    plt.close(fig)


def fig_ablation():
    """Прицельное разрушение: вклад прицельного разрушения и параллельных запусков (ILS после DSatur)."""
    rnd = [int(x["score"]) for x in rows("ablation_random.csv")]
    tgt = [int(x["score"]) for x in rows("ablation_targeted.csv")]
    ms = [int(x["score"]) for x in rows("multistart.csv") if x["construct"] == "dsatur"]
    groups = [("Случайный толчок,\n1 запуск", rnd), ("Прицельное разрушение,\n1 запуск", tgt),
              ("Прицельное разрушение,\n4 запуска", ms)]
    fig, ax = plt.subplots(figsize=(6.3, 3.0))
    dot_rows(ax, groups, [BLUE] * 3)
    ax.set_xlim(0, 40_000)
    ax.text(LOWER_BOUND, -0.55, " L = 12 000", color=INK2, fontsize=8, va="bottom")
    ax.set_xlabel("Критерий F (меньше — лучше), бюджет 45 с")
    fig.savefig(HERE / "targeted_ruin.png")
    plt.close(fig)


if __name__ == "__main__":
    fig_budget()
    fig_methods()
    fig_ablation()
    print("готово:", ", ".join(sorted(p.name for p in HERE.glob("*.png"))))
