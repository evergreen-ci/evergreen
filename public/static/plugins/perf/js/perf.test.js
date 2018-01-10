describe('average', function() {
  it("should correctly find an average", function() {
    expect( average([0, 100, 25, 75]) ).toBe(50);
  });
});
