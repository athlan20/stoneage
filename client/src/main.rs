use std::error::Error;
use std::io;
use std::time::{Duration, Instant};

use clap::{Parser, Subcommand};
use crossterm::event::{self, Event};
use crossterm::execute;
use crossterm::terminal::{EnterAlternateScreen, LeaveAlternateScreen, disable_raw_mode, enable_raw_mode};
use ratatui::Terminal;
use ratatui::backend::{Backend, CrosstermBackend};
use stoneage_client::app::{App, DEFAULT_SERVER, Options};
use stoneage_client::console_mode;
use stoneage_client::ui;
use stoneage_client::update;

/// 石器时代 命令行客户端。密码只能通过环境变量 STONEAGE_PASSWORD 传入。
#[derive(Debug, Parser)]
#[command(version)]
struct Cli {
    #[command(subcommand)]
    command: Option<Command>,
    /// 服务端地址
    #[arg(long, default_value = DEFAULT_SERVER)]
    server: String,
    /// 预先填好账号
    #[arg(long)]
    account: Option<String>,
    /// 登录后自动进入指定角色（名字或栏位序号，序号从 1 起）
    #[arg(long)]
    character: Option<String>,
    /// 图形字符开关：true 用 Braille 与粗线，false 退回 Dot 与细线
    #[arg(long, default_value_t = true, action = clap::ArgAction::Set)]
    unicode: bool,
    /// 启动时不在后台查新版本（离线或内网使用；也可设环境变量 STONEAGE_NO_UPDATE_CHECK）
    #[arg(long)]
    no_update_check: bool,
    /// 减少动态效果：不画卡片位移和飞行物，只留出手者、方向和接触结果（适合低刷新终端）
    #[arg(long)]
    reduce_motion: bool,
}

#[derive(Debug, Subcommand)]
enum Command {
    /// 升级到最新版本（不进游戏界面）
    Update,
}

fn main() -> Result<(), Box<dyn Error>> {
    update::cleanup_after_upgrade();
    let cli = Cli::parse();
    if let Some(Command::Update) = cli.command {
        if let Err(err) = update::command::run() {
            eprintln!("{err}");
            std::process::exit(1);
        }
        return Ok(());
    }
    let opts = Options {
        update_check: !cli.no_update_check && !update::disabled_by_env(),
        server: cli.server,
        account: cli.account,
        password: std::env::var("STONEAGE_PASSWORD").ok().filter(|p| !p.is_empty()),
        character: cli.character,
        unicode: cli.unicode,
        reduce_motion: cli.reduce_motion,
    };

    // 无论怎么退出都要恢复终端。
    let default_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let _ = restore_terminal();
        default_hook(info);
    }));

    console_mode::drop_vt_and_mouse_input();
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout))?;
    let result = run(&mut terminal, App::new(opts));
    restore_terminal()?;
    terminal.show_cursor()?;
    if let Err(err) = result {
        eprintln!("{err:?}");
    }
    Ok(())
}

fn restore_terminal() -> io::Result<()> {
    disable_raw_mode()?;
    console_mode::restore_input_mode();
    execute!(io::stdout(), LeaveAlternateScreen)
}

fn run<B: Backend>(terminal: &mut Terminal<B>, mut app: App) -> Result<(), Box<dyn Error>>
where
    B::Error: 'static,
{
    let poll_slice = Duration::from_millis(50);
    let mut last_tick = Instant::now();
    while !app.should_quit {
        terminal.draw(|frame| ui::render(frame, &mut app))?;
        if event::poll(poll_slice.min(app.tick_rate()))? {
            if let Event::Key(key) = event::read()? {
                app.on_key(key);
            }
        }
        app.pump_net();
        if let Some(text) = app.take_clipboard() {
            stoneage_client::clipboard::copy(&text);
        }
        if last_tick.elapsed() >= app.tick_rate() {
            app.on_tick(Instant::now());
            last_tick = Instant::now();
        }
    }
    Ok(())
}
