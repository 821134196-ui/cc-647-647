#!/usr/bin/env python3
"""端到端冒烟验证：漏记复核、并列重赛、岗位权限、榜单版本化。"""
import json
import urllib.request
import urllib.error

BASE = "http://localhost:8080"
TOK = {}


def call(method, path, role=None, body=None, expect=None):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if role:
        req.add_header("Authorization", "Bearer " + TOK[role])
    try:
        with urllib.request.urlopen(req) as r:
            code, text = r.status, r.read()
    except urllib.error.HTTPError as e:
        code, text = e.code, e.read()
    out = json.loads(text) if text else None
    tag = f"{method} {path}"
    if expect and code != expect:
        raise AssertionError(f"{tag} 期望 {expect}，实际 {code}：{text.decode()[:200]}")
    print(f"  [{code}] {tag}" + (f" -> {out.get('error') or out.get('message') or ''}" if isinstance(out, dict) else ""))
    return code, out


# 1. 登录
for role, user in [("clerk", "clerk"), ("judge", "judge"), ("chief", "chief")]:
    _, out = call("POST", "/api/auth/login", body={"username": user, "password": user + "123"}, expect=200)
    TOK[role] = out["token"]
_, out = call("POST", "/api/auth/login", body={"username": "x", "password": "y"}, expect=401)

# 2. 找项目
_, meets = call("GET", "/api/meets", "chief")
race_id = {r["code"]: r["id"] for g in meets for r in g["races"]}
rid = race_id["M101"]


def lanes_of(role="chief"):
    _, d = call("GET", f"/api/races/{rid}", role)
    return {l["entry"]["lane_no"]: l for l in d["lanes"]}


print("\n== 场景一：漏记复核 ==")
call("POST", f"/api/races/{rid}/readings/import", "clerk", {}, 200)
lanes = lanes_of()
l3 = lanes[3]
assert l3["latest_reading"]["status"] == "MISSED" and l3["latest_reading"]["finish_time"] is None
assert l3["rank"] is None and "MISSED" in l3["flags"]
print("  ✓ 3 道漏记：无触壁时间、无名次、标 MISSED")
call("POST", f"/api/races/{rid}/boards", "chief", {}, 400)  # 漏记未决禁止发布
print("  ✓ 漏记未裁定时禁止发布榜单")

# 录入员越权：手记成绩 403、发起复核 403
call("POST", f"/api/races/{rid}/lanes/{l3['entry']['id']}/notes", "clerk",
     {"content": "越权尝试", "manual_finish": 55.67}, 403)
call("POST", f"/api/races/{rid}/lanes/{l3['entry']['id']}/notes", "clerk",
     {"content": "终点录像已存档"}, 200)
call("POST", f"/api/races/{rid}/cases", "clerk",
     {"lane_entry_id": l3["entry"]["id"], "reason_type": "MISSED_TOUCH", "summary": "x"}, 403)
print("  ✓ 录入员：手记成绩被拒、可补文字材料、不能发起复核")

# 裁判手记 + 复核；裁判不能裁定
call("POST", f"/api/races/{rid}/lanes/{l3['entry']['id']}/notes", "judge",
     {"content": "三块边道秒表 55.66/55.68/55.67", "manual_finish": 55.67}, 200)
_, out = call("POST", f"/api/races/{rid}/cases", "judge",
              {"lane_entry_id": l3["entry"]["id"], "reason_type": "MISSED_TOUCH",
               "summary": "3 道触板无响应"}, 200)
cid = out["case"]["id"]
call("POST", f"/api/cases/{cid}/rulings", "judge",
     {"decision": "USE_MANUAL", "rationale": "越权"}, 403)
print("  ✓ 裁判：可手记、可发起复核；不能终局裁定")

# 总裁判手动裁定（不带成绩 → 取最新手记 55.67）
call("POST", f"/api/cases/{cid}/rulings", "chief",
     {"decision": "USE_MANUAL", "rationale": "录像与边道秒表一致，触板漏记"}, 200)
l3 = lanes_of()[3]
assert l3["source"] == "MANUAL" and l3["resolved_seconds"] == 55.67
assert l3["rank"] == 4 and "MISSED" not in l3["flags"]
print(f"  ✓ 总裁判裁定手动成绩：55.67，名次第 {l3['rank']}，MISSED 标记解除")

# 发布初版榜单（此时 5/6 道仍并列、无人撤回）
_, out = call("POST", f"/api/races/{rid}/boards", "chief", {}, 200)
v1 = out["board_id"]
print(f"  ✓ 初版榜单 V{v1 if False else out['version_no']} 发布")

print("\n== 场景二：并列排序与重赛 ==")
lanes = lanes_of()
assert lanes[5]["rank"] == lanes[6]["rank"] == 2 and lanes[1]["rank"] == 5 and lanes[8]["rank"] == 7
print("  ✓ 5/6 道并列第 2，标准竞赛排名 1-2-2-4-5-6-7-8（3 道手动 55.67 占第 4）")
_, out = call("POST", f"/api/races/{rid}/cases", "judge",
              {"lane_entry_id": 0, "reason_type": "TIE_RESOLVE", "summary": "5、6 道成绩相同"}, 200)
tie_cid = out["case"]["id"]
_, out = call("POST", f"/api/cases/{tie_cid}/rulings", "chief",
              {"decision": "USE_SWIMOFF", "rationale": "并列，安排重赛",
               "lane_entry_ids": [lanes[5]["entry"]["id"], lanes[6]["entry"]["id"]]}, 200)
_, meets = call("GET", "/api/meets", "chief")
so_id = {r["code"]: r["id"] for g in meets for r in g["races"]}["M101-SO1"]
print("  ✓ 重赛项目 M101-SO1 已生成")
# 重赛未导入：回退显示电子成绩
l5 = lanes_of()[5]
assert l5["source"] == "ELECTRONIC" and l5["note"] == "并列重赛待进行"
print("  ✓ 重赛未进行时回退显示电子成绩并提示")
call("POST", f"/api/races/{so_id}/readings/import", "clerk", {}, 200)
lanes = lanes_of()
l5, l6 = lanes[5], lanes[6]
assert l5["source"] == l6["source"] == "SWIMOFF"
assert l5["resolved_seconds"] == 55.14 and l6["resolved_seconds"] == 55.48
assert l5["rank"] != l6["rank"] and "TIE" not in l5["flags"] and "SWIMOFF" in l5["flags"]
print(f"  ✓ 重赛成绩 55.14 / 55.48，分出第 {l5['rank']}、{l6['rank']} 名")

print("\n== 场景三：撤回与榜单版本化 ==")
call("POST", f"/api/races/{rid}/lanes/{lanes[7]['entry']['id']}/withdraw", "judge",
     {"reason": "裁判不能撤回"}, 403)
call("POST", f"/api/races/{rid}/lanes/{lanes[7]['entry']['id']}/withdraw", "chief",
     {"reason": "检录点名三次未到"}, 200)
print("  ✓ 裁判不能撤回，总裁判可以")

call("POST", f"/api/races/{rid}/boards", "chief", {}, 400)  # 更正必须填原因
_, out = call("POST", f"/api/races/{rid}/boards", "chief",
              {"correction_reason": "3 道漏记改手动；5/6 道重赛决名次；7 道弃权"}, 200)
v2 = out["board_id"]
_, ver = call("GET", f"/api/races/{rid}/boards", "clerk")
status = {b["version_no"]: b["status"] for b in ver}
assert status == {1: "SUPERSEDED", 2: "CURRENT"}, status
print("  ✓ V1 发布 → 更正需原因 → V2 发布，V1 置 SUPERSEDED")

_, b1 = call("GET", f"/api/boards/{v1}", "clerk")
_, b2 = call("GET", f"/api/boards/{v2}", "clerk")
e1 = {e["lane_no"]: e for e in b1["board"]["entries"]}
e2 = {e["lane_no"]: e for e in b2["board"]["entries"]}
assert e1[3]["source"] == "MANUAL" and e1[5]["rank"] == 2 and e1[5]["flags"] == "TIE"
assert e1[7]["rank"] is not None, "V1 快照里 7 道应有成绩"
assert e2[7]["rank"] is None and e2[7]["flags"] == "WITHDRAWN"
assert e2[5]["source"] == "SWIMOFF" and e2[5]["rank"] != e2[6]["rank"]
assert b2["board"]["correction_reason"].startswith("3 道漏记改手动")
print("  ✓ V1 旧快照不被覆盖（7 道仍有成绩、5 道仍并列）；V2 为撤回/重赛后更正版并公示原因")

print("\n== 场景四：原始读数批次不覆盖（M102 设备补发）=")
rid2 = race_id["M102"]
call("POST", f"/api/races/{rid2}/readings/import", "clerk", {}, 200)
call("POST", "/api/device/sessions/M102/retransmit", "clerk",
     {"lane": 4, "finish_time": 151.9}, 200)
call("POST", f"/api/races/{rid2}/readings/import", "clerk", {}, 200)
_, raw = call("GET", f"/api/races/{rid2}/readings", "clerk")
rows = [r for r in raw["readings"] if r["lane_no"] == 4]
assert len(rows) == 2 and rows[0]["status"] == "PARTIAL" and rows[0]["finish_time"] is None
assert rows[1]["status"] == "OK" and rows[1]["finish_time"] == 151.9
print("  ✓ 两批读数并存：第1批 PARTIAL 漏记原样保留，第2批 OK 151.90")

print("\n🎉 全部端到端验证通过")
