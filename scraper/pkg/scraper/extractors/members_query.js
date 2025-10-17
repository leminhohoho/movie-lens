$('#content > div > div > section > table > tbody > tr')
    .get()
    .map((row, _) => ({
        name: $(row).find('td > div > h3 > a').text().trim(),
        url: $(row).find('td > div > h3 > a').attr('href'),
    }))
