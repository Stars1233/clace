
# Declarative app config for Streamlit apps, install by running
#    clace apply --approve github.com/claceio/clace/examples/streamlit.star

sl_args = {"container_opts": {"cpus": "2", "memory": "512m"}, "spec": "python-streamlit"}
app("/streamlit/spirals", "github.com/streamlit/streamlit-example", git_branch="master", params={"app_name": "Spirals"}, **sl_args)
app("/streamlit/uber", "github.com/streamlit/demo-uber-nyc-pickups", params={"app_name": "NYC Uber Ridesharing Data"}, **sl_args)
app("/streamlit/echarts", "github.com/andfanilo/streamlit-echarts-demo", git_branch="master",
    params={"app_file": "app.py", "app_name": "ECharts Demo"}, **sl_args)
app("/streamlit/snowflake", "github.com/syasini/snowflake_cheatsheet",
    params={"app_file": "app.py", "app_name": "Snowflake Cheatsheet"}, **sl_args)
app("maps.:", "github.com/chrieke/prettymapp", # uses default domain, eg maps.localhost:/
    params={"app_file": "streamlit-prettymapp/app.py", "app_name": "Prettymapp"}, **sl_args)
