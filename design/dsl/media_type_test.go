package dsl_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/raphael/goa/design"
	. "github.com/raphael/goa/design/dsl"
)

var _ = Describe("MediaType", func() {
	var name string
	var dsl func()

	var mt *MediaTypeDefinition

	BeforeEach(func() {
		Design = nil
		Errors = nil
		name = ""
		dsl = nil
	})

	JustBeforeEach(func() {
		mt = MediaType(name, dsl)
		RunDSL()
		Ω(Errors).ShouldNot(HaveOccurred())
	})

	Context("with no DSL and no identifier", func() {
		It("produces an error", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).Should(HaveOccurred())
		})
	})

	Context("with no DSL", func() {
		BeforeEach(func() {
			name = "foo"
		})

		It("produces an error", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).Should(HaveOccurred())
		})
	})

	Context("with attributes", func() {
		const attName = "att"

		BeforeEach(func() {
			name = "foo"
			dsl = func() {
				Attributes(func() {
					Attribute(attName)
				})
				View("default", func() { Attribute(attName) })
			}
		})

		It("sets the attributes", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).ShouldNot(HaveOccurred())
			Ω(mt.AttributeDefinition).ShouldNot(BeNil())
			Ω(mt.Type).Should(BeAssignableToTypeOf(Object{}))
			o := mt.Type.(Object)
			Ω(o).Should(HaveLen(1))
			Ω(o).Should(HaveKey(attName))
		})
	})

	Context("with a description", func() {
		const description = "desc"

		BeforeEach(func() {
			name = "foo"
			dsl = func() {
				Description(description)
				Attributes(func() {
					Attribute("attName")
				})
				View("default", func() { Attribute("attName") })
			}
		})

		It("sets the description", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).ShouldNot(HaveOccurred())
			Ω(mt.Description).Should(Equal(description))
		})
	})

	Context("with links", func() {
		const linkName = "link"
		var link1Name, link2Name string
		var link2View string
		var linkedMT1, linkedMT2 *MediaTypeDefinition

		BeforeEach(func() {
			name = "foo"
			link1Name = "l1"
			link2Name = "l2"
			link2View = "l2v"
			linkedMT1 = NewMediaTypeDefinition("MT1", "MT1", func() {
				Attributes(func() {
					Attribute("foo")
				})
				View("default", func() {
					Attribute("foo")
				})
				View("link", func() {
					Attribute("foo")
				})
			})
			InitDesign()
			linkedMT2 = NewMediaTypeDefinition("MT2", "MT2", func() {
				Attributes(func() {
					Attribute("foo")
				})
				View("l2v", func() {
					Attribute("foo")
				})
				View("default", func() {
					Attribute("foo")
				})
			})
			Design.MediaTypes = make(map[string]*MediaTypeDefinition)
			Design.MediaTypes["MT1"] = linkedMT1
			Design.MediaTypes["MT2"] = linkedMT2
			dsl = func() {
				Attributes(func() {
					Attributes(func() {
						Attribute(link1Name, linkedMT1)
						Attribute(link2Name, linkedMT2)
					})
					Links(func() {
						Link(link1Name)
						Link(link2Name, link2View)
					})
					View("default", func() {
						Attribute(link1Name)
						Attribute(link2Name)
					})
				})
			}
		})

		It("sets the links", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).ShouldNot(HaveOccurred())
			Ω(mt.Links).ShouldNot(BeNil())
			Ω(mt.Links).Should(HaveLen(2))
			Ω(mt.Links).Should(HaveKey(link1Name))
			Ω(mt.Links[link1Name].Name).Should(Equal(link1Name))
			Ω(mt.Links[link1Name].View).Should(Equal("link"))
			Ω(mt.Links[link1Name].Parent).Should(Equal(mt))
			Ω(mt.Links[link2Name].Name).Should(Equal(link2Name))
			Ω(mt.Links[link2Name].View).Should(Equal(link2View))
			Ω(mt.Links[link2Name].Parent).Should(Equal(mt))
		})
	})

	Context("with views", func() {
		const viewName = "view"
		const viewAtt = "att"

		BeforeEach(func() {
			name = "foo"
			dsl = func() {
				Attributes(func() {
					Attribute(viewAtt)
				})
				View(viewName, func() {
					Attribute(viewAtt)
				})
				View("default", func() {
					Attribute(viewAtt)
				})
			}
		})

		It("sets the views", func() {
			Ω(mt).ShouldNot(BeNil())
			Ω(mt.Validate()).ShouldNot(HaveOccurred())
			Ω(mt.Views).ShouldNot(BeNil())
			Ω(mt.Views).Should(HaveLen(2))
			Ω(mt.Views).Should(HaveKey(viewName))
			v := mt.Views[viewName]
			Ω(v.Name).Should(Equal(viewName))
			Ω(v.Parent).Should(Equal(mt))
			Ω(v.AttributeDefinition).ShouldNot(BeNil())
			Ω(v.AttributeDefinition.Type).Should(BeAssignableToTypeOf(Object{}))
			o := v.AttributeDefinition.Type.(Object)
			Ω(o).Should(HaveLen(1))
			Ω(o).Should(HaveKey(viewAtt))
			Ω(o[viewAtt]).ShouldNot(BeNil())
			Ω(o[viewAtt].Type).Should(Equal(String))
		})
	})
})

var _ = Describe("CollectionOf", func() {
	Context("used on a global variable", func() {
		var col *MediaTypeDefinition
		BeforeEach(func() {
			Design = nil
			mt := MediaType("MT", func() { Attribute("id") })
			col = CollectionOf(mt)
		})

		JustBeforeEach(func() {
			RunDSL()
			Ω(Errors).ShouldNot(HaveOccurred())
		})

		It("produces a media type", func() {
			Ω(col).ShouldNot(BeNil())
			Ω(col.Identifier).ShouldNot(BeEmpty())
			Ω(col.TypeName).ShouldNot(BeEmpty())
			Ω(Design.MediaTypes).Should(HaveKey(col.Identifier))
		})
	})
})
