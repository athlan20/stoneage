//! 词条详情：把快照里的字段、表格、备注、相关词条转成终端能直接显示的纯文本行。
//! 网页版的图片、素材下载和外部来源在终端里没用，不转（`docs/game-wiki.md`）。

use serde::{Deserialize, Deserializer};

/// 快照里空列表有时写成 `null`，按空处理。
fn nullable<'de, D: Deserializer<'de>, T: Default + Deserialize<'de>>(d: D) -> Result<T, D::Error> {
    Ok(Option::<T>::deserialize(d)?.unwrap_or_default())
}

#[derive(Debug, Clone, Deserialize)]
pub struct Field {
    pub label: String,
    pub value: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Table {
    #[serde(default)]
    pub title: String,
    #[serde(default, deserialize_with = "nullable")]
    pub columns: Vec<String>,
    #[serde(default, deserialize_with = "nullable")]
    pub rows: Vec<Vec<String>>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Link {
    #[serde(default)]
    pub key: String,
    #[serde(default)]
    pub name: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct Entry {
    pub key: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub group: String,
    #[serde(default, deserialize_with = "nullable")]
    pub fields: Vec<Field>,
    #[serde(default, deserialize_with = "nullable")]
    pub tables: Vec<Table>,
    #[serde(default, deserialize_with = "nullable")]
    pub notes: Vec<String>,
    #[serde(default, deserialize_with = "nullable")]
    pub links: Vec<Link>,
}

/// 详情行的样式类别，界面按它配色。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LineStyle {
    /// 词条名。
    Title,
    /// 小节标题（表格名、备注、相关词条）。
    Section,
    /// 表头。
    Header,
    /// 正文。
    Body,
    /// 次要信息（相关词条的 key）。
    Dim,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DetailLine {
    pub style: LineStyle,
    pub text: String,
}

fn line(style: LineStyle, text: impl Into<String>) -> DetailLine {
    DetailLine { style, text: text.into() }
}

/// 网页版里的“原始名称”是 GBK 字节误读出来的乱码，终端里不显示。
const HIDDEN_FIELDS: [&str; 1] = ["原始名称"];

impl Entry {
    /// 详情的文本行。`unicode` 决定小节分隔和表格列分隔用画线字符还是 ASCII。
    pub fn detail_lines(&self, unicode: bool) -> Vec<DetailLine> {
        let (bar, col) = if unicode { ("──", " │ ") } else { ("--", " | ") };
        let mut out = vec![line(LineStyle::Title, self.name.clone())];
        if !self.description.is_empty() {
            out.push(line(LineStyle::Body, self.description.clone()));
        }
        if !self.group.is_empty() {
            out.push(line(LineStyle::Body, format!("分组：{}", self.group)));
        }
        out.push(line(LineStyle::Dim, format!("编号：{}", self.key)));
        let fields: Vec<_> = self.fields.iter().filter(|f| !HIDDEN_FIELDS.contains(&f.label.as_str())).collect();
        if !fields.is_empty() {
            out.push(line(LineStyle::Body, ""));
            for f in fields {
                for (i, v) in f.value.lines().enumerate() {
                    out.push(line(LineStyle::Body, if i == 0 { format!("{}：{v}", f.label) } else { format!("  {v}") }));
                }
                if f.value.is_empty() {
                    out.push(line(LineStyle::Body, format!("{}：", f.label)));
                }
            }
        }
        for t in &self.tables {
            out.push(line(LineStyle::Body, ""));
            out.push(line(LineStyle::Section, format!("{bar} {} {bar}", t.title)));
            if !t.columns.is_empty() {
                out.push(line(LineStyle::Header, t.columns.join(col)));
            }
            for r in &t.rows {
                out.push(line(LineStyle::Body, r.join(col).replace('\n', " ")));
            }
        }
        if !self.notes.is_empty() {
            out.push(line(LineStyle::Body, ""));
            out.push(line(LineStyle::Section, format!("{bar} 备注 {bar}")));
            for n in &self.notes {
                out.push(line(LineStyle::Body, format!("· {n}")));
            }
        }
        let links: Vec<_> = self.links.iter().filter(|l| !l.key.is_empty()).collect();
        if !links.is_empty() {
            out.push(line(LineStyle::Body, ""));
            out.push(line(LineStyle::Section, format!("{bar} 相关词条（wiki open 编号）{bar}")));
            for l in links {
                out.push(line(LineStyle::Body, format!("{}  {}", l.key, l.name)));
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry(json: &str) -> Entry {
        serde_json::from_str(json).unwrap()
    }

    #[test]
    fn null_lists_are_empty() {
        let e = entry(r#"{"key":"guide:x","name":"指南","fields":null,"tables":null,"notes":null,"links":null}"#);
        let lines = e.detail_lines(true);
        assert_eq!(lines[0], DetailLine { style: LineStyle::Title, text: "指南".into() });
        assert_eq!(lines.last().unwrap().text, "编号：guide:x");
    }

    #[test]
    fn renders_fields_tables_notes_and_links() {
        let e = entry(
            r#"{"key":"pet:1","name":"乌力","description":"基础模板","fields":[{"label":"原始名称","value":"乱码"},{"label":"等级","value":"5"}],
               "tables":[{"title":"技能","columns":["名","编号"],"rows":[["攻击","1"],["防御","2"]]}],
               "notes":["仅供参考"],"links":[{"key":"map:100","name":"某地"},{"name":"外部","url":"https://x"}]}"#,
        );
        let text: Vec<String> = e.detail_lines(false).into_iter().map(|l| l.text).collect();
        assert!(!text.iter().any(|t| t.contains("乱码")), "原始名称不显示");
        assert!(text.contains(&"等级：5".to_string()));
        assert!(text.contains(&"-- 技能 --".to_string()));
        assert!(text.contains(&"名 | 编号".to_string()), "ASCII 降级用竖线分列");
        assert!(text.contains(&"攻击 | 1".to_string()));
        assert!(text.contains(&"· 仅供参考".to_string()));
        assert!(text.contains(&"map:100  某地".to_string()));
        assert!(!text.iter().any(|t| t.contains("外部")), "没有 key 的外部链接不显示");
        let uni: Vec<String> = e.detail_lines(true).into_iter().map(|l| l.text).collect();
        assert!(uni.contains(&"名 │ 编号".to_string()));
    }

    #[test]
    fn multi_line_field_values_are_indented() {
        let e = entry(r#"{"key":"a:1","name":"n","fields":[{"label":"效果","value":"第一行\n第二行"}]}"#);
        let text: Vec<String> = e.detail_lines(true).into_iter().map(|l| l.text).collect();
        assert!(text.contains(&"效果：第一行".to_string()) && text.contains(&"  第二行".to_string()));
    }
}
