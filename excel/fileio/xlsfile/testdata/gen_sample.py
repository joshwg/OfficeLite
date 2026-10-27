"""Regenerates sample.xls, the BIFF8 fixture for xlsfile tests.

    python3 -m venv /tmp/xlwt-venv && /tmp/xlwt-venv/bin/pip install xlwt
    /tmp/xlwt-venv/bin/python gen_sample.py
"""
import datetime
import os

import xlwt

wb = xlwt.Workbook()
ws = wb.add_sheet("Data")

bold_red = xlwt.easyxf("font: bold on, color red, height 280, name Arial; align: horiz center")
money = xlwt.easyxf(num_format_str="#,##0.00")
date = xlwt.easyxf(num_format_str="yyyy-mm-dd")
filled = xlwt.easyxf("pattern: pattern solid, fore_colour yellow; borders: left thin, right thin, top thin, bottom medium")

ws.write(0, 0, "Title", bold_red)
ws.write(1, 0, 1234.5, money)
ws.write(1, 1, 42)  # integer: stored as RK
ws.write(1, 2, 0.1)  # float: stored as NUMBER
ws.write(2, 0, datetime.date(2026, 10, 15), date)
ws.write(2, 1, True)
ws.write(3, 0, "boxed", filled)
ws.write(4, 0, "Ünïcödé ✓")
ws.write(5, 0, "x" * 9000)  # forces the shared string table to CONTINUE
ws.write_merge(6, 7, 0, 2, "merged")
ws.col(0).width = 256 * 20
ws.row(8).height_mismatch = True
ws.row(8).height = 20 * 30
ws.write(8, 0, "tall")
ws.set_panes_frozen(True)
ws.set_horz_split_pos(1)
ws.set_vert_split_pos(1)

hidden = wb.add_sheet("Second")
for i in range(300):
    hidden.write(i, 0, "row %d" % i)

wb.save(os.path.join(os.path.dirname(os.path.abspath(__file__)), "sample.xls"))
