
function showLogs() {
    var xhr=new XMLHttpRequest();

    xhr.onreadystatechange=function () {
        if (xhr.readyState==4){
            var filelist = new Array();
            filelist = xhr.responseText.split("\r\n");
            var select = document.getElementById('files');
            var options = "<option>==select one==</option>";

            for(var i=0; i<filelist.length; i++){
                options +='<option>'+filelist[i]+'</option>';
            }
            select.innerHTML = options;
        }
    };
    xhr.open('get','http://127.0.0.1:2222/selectLogs');
    xhr.send(null);
}

function openLogfile() {

    var select = document.getElementById('files');
    if (select.options.length == 0)
    {
        alert("请先获取文件列表并选择文件！");
        return false;
    }
    var index = select.selectedIndex;
    var file = select.options[index].text;
    if (file == "==select one==")
    {
        alert("请选择文件！");
        return false;
    }
    var xhr=new XMLHttpRequest();
    xhr.onreadystatechange=function () {
        if (xhr.readyState==4){
            document.getElementById("showBox").innerHTML = xhr.responseText;
        }
    };
    xhr.open('get','http://127.0.0.1:2222/readlogfile?name='+file);
    xhr.send(null);
}