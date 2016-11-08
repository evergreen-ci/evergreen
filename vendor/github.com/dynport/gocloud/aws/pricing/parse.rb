require "nokogiri"

data = File.read("fixtures/instance_types.html")

doc = Nokogiri::HTML(data)


tables = []
doc.css("table").each do |table|
  rows = []
  table.search("tr").each do |tr|
    row = []
    tr.search("td").each do |td|
      row << td.inner_text.gsub(/\s+/, " ").strip
    end
    rows << row
  end
  tables << rows
end

tables.each do |table|
  table.each do |row|
    puts row[1..-1].join("\t")
  end
  break
end

puts "-" * 100
