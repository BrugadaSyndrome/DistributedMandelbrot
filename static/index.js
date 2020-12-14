document.getElementById('reset_settings').addEventListener('click', function (event) {
    event.preventDefault();
    fetch("http://localhost:8080/defaultSettings")
        .then(function (response) {
            return response.json()
        })
        .then(function (data) {
            Object.keys(data).forEach(function (key) {
                let element = document.getElementById(key);
                if (element == null) {
                    return;
                }
                switch (element.type) {
                    case 'number':
                        element.value = data[key];
                        break;
                    case 'checkbox':
                        let boolValue = (data[key] === true || data[key] === 'true')
                        if (element.checked !== boolValue) {
                            element.click();
                        }
                        break;
                    case 'file':
                        // todo: handle this case...
                        break;
                    default:
                        break;
                }
            });
        });
});

document.getElementById('submit_settings').addEventListener('click', function (event) {
    event.preventDefault();
    let settings = {
        Width: document.getElementById('width').value,
        Height: document.getElementById('height').value,
        CenterX: document.getElementById('centerX').value,
        CenterY: document.getElementById('centerY').value,
        MagnificationStart: document.getElementById('magnificationStart').value,
        MagnificationEnd: document.getElementById('magnificationEnd').value,
        MagnificationStep: document.getElementById('magnificationStep').value,
        SmoothColoring: (document.getElementById('smoothColoring').checked === 'true'),
        Boundary: document.getElementById('boundary').value,
        MaxIterations: document.getElementById('maxIterations').value,
    };
    console.log(settings);
    fetch("http://localhost:8080/settings")
        .then(function (response) {
            console.log('first get response: ', response.json());
        })
        .then(function (data) {
            console.log('first get data: ', data);
        });

    fetch("http://localhost:8080/settings", {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(settings)
    })
        .then(function (response) {
            console.log('post response: ', response);
        });

    fetch("http://localhost:8080/settings")
        .then(function (response) {
            console.log('second get response: ', response.json());
        })
        .then(function (data) {
            console.log('second get data: ', data);
        });
    // document.getElementById('tab-2').click();
});