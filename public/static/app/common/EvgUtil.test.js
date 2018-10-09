describe('MDBQueryAdaptorSpec', function() {
  beforeEach(module('MCI'));

  var svc

  beforeEach(inject(function($injector) {
    svc = $injector.get('EvgUtil')
  }))

  it('generates versionId', function() {
    expect(
      svc.generateVersionId({project: 'sys-perf', revision: 'abc123'})
    ).toEqual('sys_perf_abc123')
  })

  it('generates buildId', function() {
    expect(
      svc.generateBuildId({
        project: 'sys-perf',
        revision: 'abc123',
        buildVariant: 'linux-1-node-replSet',
        createTime: '2018-10-06T14:43:50Z'
      })
    ).toEqual('sys_perf_linux_1_node_replSet_abc123_18_10_06_14_43_50')
  })
})
